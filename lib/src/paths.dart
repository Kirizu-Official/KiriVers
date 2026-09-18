/// Native JSON routes the handwritten client implements.
///
/// Store feeds and leftover unregistered paths are listed only so tests can
/// assert they are never emitted.
class NativeRoutes {
  NativeRoutes._();

  static const health = '/api/v1/health';
  static const project = '/api/v1/projects/{project_ref}';
  static const deviceReport = '/api/v1/projects/{project_ref}/clients/report';
  static const check = '/api/v1/projects/{project_ref}/update/check';
  static const changelog =
      '/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}';
  static const integrity =
      '/api/v1/projects/{project_ref}/versions/{version}/integrity';
  static const diff = '/api/v1/projects/{project_ref}/update/diff';
  static const pack = '/api/v1/projects/{project_ref}/update/pack';
  static const packages = '/api/v1/projects/{project_ref}/packages/{ref}';
  static const channels = '/api/v1/projects/{project_ref}/channels';
  static const matrix = '/api/v1/projects/{project_ref}/matrix';
  static const languages = '/api/v1/projects/{project_ref}/languages';
  static const announcements = '/api/v1/projects/{project_ref}/announcements';
  static const telemetry = '/api/v1/projects/{project_ref}/telemetry/report';
  static const media = '/api/v1/projects/{project_ref}/media/{id}';

  /// METHOD + space + OpenAPI path template.
  static const inSdk = <String>[
    'GET $health',
    'GET $project',
    'POST $deviceReport',
    'POST $check',
    'GET $changelog',
    'GET $integrity',
    'POST $diff',
    'POST $pack',
    'GET $packages',
    'HEAD $packages',
    'GET $channels',
    'GET $matrix',
    'GET $languages',
    'GET $announcements',
    'POST $telemetry',
    'GET $media',
    'HEAD $media',
  ];

  static const outOfSdk = <String>[
    'GET /api/v1/projects/{project_ref}/update/check',
    'POST /api/v1/projects/{project_ref}/clients/login',
    'GET /api/v1/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/manifest',
    'POST /api/v1/projects/{project_ref}/update/pack/status',
    'GET /api/v1/projects/{project_ref}/channels/{slug}',
    'GET /api/v1/projects/{project_ref}/artifacts/{artifact_id}/{filename}',
    'HEAD /api/v1/projects/{project_ref}/artifacts/{artifact_id}/{filename}',
    'GET /api/v1/ready',
    'GET /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}',
    'GET /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}/{doc}',
  ];

  static bool matchesTemplate(String actualPath, String template) {
    final actual = Uri.parse(actualPath).path;
    final t = template.split('/');
    final a = actual.split('/');
    if (t.length != a.length) {
      return false;
    }
    for (var i = 0; i < t.length; i++) {
      final seg = t[i];
      if (seg.startsWith('{') && seg.endsWith('}')) {
        continue;
      }
      if (seg != a[i]) {
        return false;
      }
    }
    return true;
  }
}
