import 'browser_base.dart';

/// In-memory browser used by tests and by any non-web build.
class MemoryBrowser implements BrowserBridge {
  MemoryBrowser({Uri? location, this.origin = 'http://localhost'}) : location = location ?? Uri.parse('http://localhost/loader-app/');

  @override
  Uri location;
  @override
  final String origin;

  final Map<String, String> storage = {};
  final List<String> navigations = [];
  final List<String> replacements = [];

  @override
  void navigate(String url) => navigations.add(url);

  @override
  void replaceUrl(String pathAndQuery) {
    replacements.add(pathAndQuery);
    location = Uri.parse(origin).resolve(pathAndQuery);
  }

  @override
  String? sessionGet(String key) => storage[key];
  @override
  void sessionSet(String key, String value) => storage[key] = value;
  @override
  void sessionRemove(String key) => storage.remove(key);
}

BrowserBridge createBrowser() => MemoryBrowser();
