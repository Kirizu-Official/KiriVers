#include <kirivers/updater.hpp>
#include <kirivers/path.hpp>

#include "delta.hpp"

#include <filesystem>
#include <fstream>
#include <iterator>
#include <stdexcept>
#include <utility>
#include <vector>

namespace kirivers {

namespace {

std::string int_or_empty(const std::optional<std::int64_t>& v) {
  return v ? std::to_string(*v) : std::string{};
}

void write_bytes(const std::string& path, const Bytes& data) {
  std::ofstream out(std::filesystem::u8path(path), std::ios::binary | std::ios::trunc);
  if (!out) throw std::runtime_error("cannot write staged file");
  out.write(reinterpret_cast<const char*>(data.data()),
            static_cast<std::streamsize>(data.size()));
}

Bytes read_bytes(const std::string& path) {
  std::ifstream in(std::filesystem::u8path(path), std::ios::binary);
  if (!in) throw std::runtime_error("cannot read staged file");
  return Bytes((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
}

Bytes load_path(Client& client, const std::string& path) {
  if (auto store = client.file_store()) return store->read_file(path);
  return read_bytes(path);
}

std::string hash_path(Client& client, const std::string& path) {
  auto hasher = client.hasher();
  if (!hasher) throw ConfigError("Hasher is required");
  if (auto store = client.file_store()) {
    return hasher->sha256_hex(store->read_file(path));
  }
  return hasher->sha256_file(path);
}

using ZipMembers = std::vector<std::pair<std::string, std::string>>;

void maybe_unpack(Client& client, const Bytes& zip, const std::string& dest_root,
                  const ZipMembers& members) {
  auto unpacker = client.archive_unpacker();
  auto store = client.file_store();
  if (!unpacker || !store || dest_root.empty() || members.empty()) return;
  unpacker->unpack_zip(zip, *store, dest_root, members);
}

}  // namespace

void Updater::write_stage(const std::string& path, const Bytes& data) {
  if (auto store = client_.file_store()) {
    store->write_file(path, data);
    return;
  }
  write_bytes(path, data);
}

void Updater::maybe_verify_signature(const UpdateCheckBody& body) {
  auto ver = client_.signature_verifier();
  const auto& cfg = client_.config();
  if (!ver || cfg.public_key_pem.empty() || !body.signature || body.signature->empty()) {
    return;
  }
  const std::string algo = cfg.signing_algo.empty() ? "ed25519" : cfg.signing_algo;
  const std::string payload = build_check_payload(
      int_or_empty(body.version_integer), body.version_semver.value_or(""),
      body.root_hash, body.package_url, std::to_string(body.size), body.sha256);
  ver->verify(algo, cfg.public_key_pem, payload, *body.signature);
}

Bytes Updater::download_and_verify(const std::string& url, const std::string& expect_sha) {
  Bytes data = client_.download(url);
  if (auto hasher = client_.hasher()) {
    const std::string got = hasher->sha256_hex(data);
    if (!expect_sha.empty() && !equal_hex(got, expect_sha)) {
      throw ApiError(0, "SHA256_MISMATCH", "downloaded bytes do not match sha256");
    }
  } else if (!expect_sha.empty()) {
    throw ConfigError("Hasher is required to verify package sha256");
  }
  return data;
}

void Updater::send_telemetry(const UpdateRequest& req, const std::string& to_version,
                             const std::string& status, const std::string& diff_mode,
                             const std::string& err_code, const std::string& err_msg) {
  try {
    TelemetryInput t;
    t.os = req.os;
    t.arch = req.arch;
    t.channel = req.channel;
    t.from_version = req.current_version;
    t.to_version = to_version;
    t.status = status;
    t.device_id = req.device_id;
    t.diff_mode = diff_mode;
    if (!err_code.empty()) t.error_code = err_code;
    if (!err_msg.empty()) t.error_message = err_msg;
    client_.report_telemetry(t);
  } catch (...) {
    // Telemetry must never fail the update.
  }
}

UpdateResult Updater::run(const UpdateRequest& req) {
  if (req.report_device && req.device_id) {
    try {
      DeviceReportInput d;
      d.device_id = *req.device_id;
      d.version = req.current_version;
      d.os = req.os;
      d.arch = req.arch;
      d.channel = req.channel;
      d.custom = req.device_custom;
      client_.report_device(d);
    } catch (...) {
    }
  }

  CheckInput cin;
  cin.current_version = req.current_version;
  cin.os = req.os;
  cin.arch = req.arch;
  cin.channel = req.channel;
  cin.hw_rev = req.hw_rev;
  cin.os_version = req.os_version;
  cin.device_id = req.device_id;
  auto checked = client_.check(cin);

  UpdateResult out;
  out.check = checked;
  if (checked.status == CheckStatus::NotModified) {
    out.kind = UpdateKind::NotModified;
    return out;
  }
  if (checked.status == CheckStatus::NoUpdate || !checked.body) {
    out.kind = UpdateKind::NoUpdate;
    return out;
  }

  const UpdateCheckBody& body = *checked.body;
  const std::string to_ver = body.version_semver.value_or(body.file_name);
  maybe_verify_signature(body);
  send_telemetry(req, to_ver, "downloading", "full_package");

  Bytes staged;
  std::string diff_mode = "full_package";
  std::string expect_sha = body.sha256;
  std::string download_url = body.package_url;
  ZipMembers unpack_map;

  try {
    const bool can_delta = client_.patcher() && body.delta_available &&
                           !body.is_downgrade && body.package_type == "single_file";
    if (can_delta && client_.hasher()) {
      DiffInput din;
      din.source_version = req.current_version;
      din.target_version = to_ver;
      din.os = req.os;
      din.arch = req.arch;
      din.channel = req.channel;
      din.device_id = req.device_id;
      din.hw_rev = req.hw_rev;
      if (!req.install_dir.empty()) {
        try {
          din.local_sha256 = hash_path(client_, req.install_dir);
        } catch (...) {
        }
      }
      auto d = client_.diff(din);
      if (d.diff_mode == "binary_delta" && d.package_url && d.sha256 && d.delta_algo) {
        try {
          Bytes delta = client_.download(*d.package_url);
          auto magic = detect_delta_magic(delta);
          if (magic == DeltaMagic::Unknown ||
              !magic_matches_algo(magic, *d.delta_algo)) {
            throw ApiError(0, "DELTA_MAGIC_UNKNOWN",
                           "delta magic does not match delta_algo");
          }
          Bytes old_bytes;
          if (!req.install_dir.empty()) old_bytes = load_path(client_, req.install_dir);
          Bytes patched = client_.patcher()->apply(old_bytes, delta, *d.delta_algo);
          const std::string got = client_.hasher()->sha256_hex(patched);
          if (!equal_hex(got, body.sha256)) {
            throw ApiError(0, "SHA256_MISMATCH", "patched bytes do not match target");
          }
          staged = std::move(patched);
          diff_mode = "binary_delta";
          expect_sha = body.sha256;
        } catch (...) {
          staged.clear();
          diff_mode = "full_package";
          download_url = body.package_url;
          expect_sha = body.sha256;
        }
      } else if (d.diff_mode == "patch_package" && d.package_url &&
                 client_.archive_unpacker()) {
        download_url = *d.package_url;
        if (d.sha256) expect_sha = *d.sha256;
        diff_mode = "patch_package";
        for (const auto& f : d.files) {
          if (f.sha256 && f.path && !f.sha256->empty() && !f.path->empty()) {
            unpack_map.emplace_back(*f.sha256, *f.path);
          }
        }
      }
    }

    if (staged.empty() && body.package_type == "multi_file" && client_.file_store() &&
        client_.hasher() && !req.install_dir.empty()) {
      IntegrityQuery iq;
      iq.os = req.os;
      iq.arch = req.arch;
      iq.channel = req.channel;
      iq.hw_rev = req.hw_rev;
      auto man = client_.integrity(to_ver, iq);
      PackInput pin;
      pin.source_version = req.current_version;
      pin.target_version = to_ver;
      pin.os = req.os;
      pin.arch = req.arch;
      pin.channel = req.channel;
      pin.device_id = req.device_id;
      pin.hw_rev = req.hw_rev;
      auto* store = client_.file_store().get();
      for (const auto& f : man.files) {
        std::string rel;
        try {
          rel = store->normalize_path(f.path);
        } catch (...) {
          continue;
        }
        const bool keep = f.install_policy == "KEEP_IF_EXISTS";
        const std::string local = req.install_dir + "/" + rel;
        if (keep && f.sha256 && store->exists(local)) {
          try {
            if (equal_hex(hash_path(client_, local), *f.sha256)) continue;
          } catch (...) {
          }
        }
        pin.needed_paths.push_back(rel);
      }
      auto packed = client_.pack_wait(pin);
      if (packed.status == "ready" && packed.package_url) {
        download_url = *packed.package_url;
        if (packed.sha256) expect_sha = *packed.sha256;
        diff_mode = packed.diff_mode.value_or("patch_package");
        unpack_map.clear();
        for (const auto& f : packed.files) {
          if (!f.sha256 || !f.path || f.sha256->empty() || f.path->empty()) continue;
          try {
            unpack_map.emplace_back(*f.sha256, store->normalize_path(*f.path));
          } catch (...) {
          }
        }
      } else {
        download_url = body.package_url;
        expect_sha = body.sha256;
        diff_mode = "full_package";
        unpack_map.clear();
      }
    }

    if (staged.empty()) {
      staged = download_and_verify(download_url, expect_sha);
      if (diff_mode == "patch_package") {
        try {
          maybe_unpack(client_, staged, req.install_dir, unpack_map);
        } catch (...) {
          // Zip already verified; unpack is best-effort (R4 stages the package).
        }
      }
    }
  } catch (const std::exception& ex) {
    send_telemetry(req, to_ver, "failed", diff_mode, "UPDATE_FAILED", ex.what());
    throw;
  }

  if (req.stage_path.empty()) {
    throw ConfigError("UpdateRequest.stage_path is required");
  }
  write_stage(req.stage_path, staged);
  out.staged_path = req.stage_path;
  out.sha256 = client_.hasher() ? client_.hasher()->sha256_hex(staged) : expect_sha;
  out.diff_mode = diff_mode;
  out.kind = UpdateKind::Staged;

  if (auto repl = client_.replacer()) {
    try {
      send_telemetry(req, to_ver, "applying", diff_mode);
      const std::string dest = req.install_dir.empty() ? req.stage_path : req.install_dir;
      repl->replace(req.stage_path, dest);
      out.kind = UpdateKind::Applied;
      out.applied_path = dest;
      send_telemetry(req, to_ver, "installed", diff_mode);
    } catch (const std::exception& ex) {
      send_telemetry(req, to_ver, "failed", diff_mode, "REPLACE_FAILED", ex.what());
      throw;
    }
  } else {
    send_telemetry(req, to_ver, "installed", diff_mode);
  }
  return out;
}

}  // namespace kirivers
