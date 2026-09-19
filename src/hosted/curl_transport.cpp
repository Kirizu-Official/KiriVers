#include <kirivers/hosted.hpp>
#include <kirivers/error.hpp>

#include <curl/curl.h>
#include <mutex>
#include <stdexcept>
#include <string>

namespace kirivers {

namespace {

std::once_flag g_curl_once;

void ensure_curl() {
  std::call_once(g_curl_once, [] {
    curl_global_init(CURL_GLOBAL_DEFAULT);
  });
}

size_t write_body(char* ptr, size_t size, size_t nmemb, void* userdata) {
  auto* out = static_cast<Bytes*>(userdata);
  const size_t n = size * nmemb;
  out->insert(out->end(), ptr, ptr + n);
  return n;
}

size_t write_header(char* ptr, size_t size, size_t nmemb, void* userdata) {
  auto* headers = static_cast<HeaderMap*>(userdata);
  const size_t n = size * nmemb;
  std::string line(ptr, n);
  while (!line.empty() && (line.back() == '\n' || line.back() == '\r')) line.pop_back();
  auto colon = line.find(':');
  if (colon == std::string::npos) return n;
  std::string key = line.substr(0, colon);
  std::string val = line.substr(colon + 1);
  auto b = val.find_first_not_of(" \t");
  if (b != std::string::npos) val = val.substr(b);
  else val.clear();
  (*headers)[key] = val;
  return n;
}

}  // namespace

struct CurlTransport::Impl {
  CURL* easy = nullptr;
  Impl() {
    ensure_curl();
    easy = curl_easy_init();
    if (!easy) throw ConfigError("curl_easy_init failed");
  }
  ~Impl() {
    if (easy) curl_easy_cleanup(easy);
  }
};

CurlTransport::CurlTransport() : impl_(std::make_unique<Impl>()) {}
CurlTransport::~CurlTransport() = default;

HttpResponse CurlTransport::execute(const HttpRequest& req) {
  CURL* easy = impl_->easy;
  curl_easy_reset(easy);

  Bytes body;
  HeaderMap headers;
  curl_easy_setopt(easy, CURLOPT_URL, req.url.c_str());
  curl_easy_setopt(easy, CURLOPT_CUSTOMREQUEST, req.method.c_str());
  curl_easy_setopt(easy, CURLOPT_NOBODY, req.method == "HEAD" ? 1L : 0L);
  curl_easy_setopt(easy, CURLOPT_FOLLOWLOCATION, 0L);
  curl_easy_setopt(easy, CURLOPT_WRITEFUNCTION, write_body);
  curl_easy_setopt(easy, CURLOPT_WRITEDATA, &body);
  curl_easy_setopt(easy, CURLOPT_HEADERFUNCTION, write_header);
  curl_easy_setopt(easy, CURLOPT_HEADERDATA, &headers);
  curl_easy_setopt(easy, CURLOPT_USERAGENT,
                   "kirivers-client-cpp/" KIRIVERS_CLIENT_VERSION);

  struct curl_slist* slist = nullptr;
  for (const auto& kv : req.headers) {
    std::string h = kv.first + ": " + kv.second;
    slist = curl_slist_append(slist, h.c_str());
  }
  // REST JSON APIs (Gin) do not need Expect: 100-continue; it stalls POSTs.
  if (auto* extra = curl_slist_append(slist, "Expect:")) {
    slist = extra;
  }
  if (slist) curl_easy_setopt(easy, CURLOPT_HTTPHEADER, slist);

  if (!req.body.empty()) {
    curl_easy_setopt(easy, CURLOPT_POSTFIELDS,
                     reinterpret_cast<const char*>(req.body.data()));
    curl_easy_setopt(easy, CURLOPT_POSTFIELDSIZE,
                     static_cast<long>(req.body.size()));
  } else if (req.method == "POST" || req.method == "PUT" || req.method == "PATCH") {
    curl_easy_setopt(easy, CURLOPT_POSTFIELDS, "");
    curl_easy_setopt(easy, CURLOPT_POSTFIELDSIZE, 0L);
  }

  char errbuf[CURL_ERROR_SIZE];
  errbuf[0] = 0;
  curl_easy_setopt(easy, CURLOPT_ERRORBUFFER, errbuf);

  const CURLcode rc = curl_easy_perform(easy);
  if (slist) curl_slist_free_all(slist);
  if (rc != CURLE_OK) {
    std::string msg = errbuf[0] ? errbuf : curl_easy_strerror(rc);
    throw ApiError(0, "TRANSPORT_ERROR", msg);
  }

  long status = 0;
  curl_easy_getinfo(easy, CURLINFO_RESPONSE_CODE, &status);
  HttpResponse resp;
  resp.status = static_cast<int>(status);
  resp.headers = std::move(headers);
  resp.body = std::move(body);
  return resp;
}

}  // namespace kirivers
