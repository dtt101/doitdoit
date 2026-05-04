/* doitdoit web config — public values, safe to commit.
 *
 * Set `dropboxAppKey` to the App key of a Dropbox app you registered at
 *   https://www.dropbox.com/developers/apps
 *
 * Recommended app settings:
 *   - Type:        "Scoped access"
 *   - Access:      "App folder"  (creates /Apps/<your-app-name>/ automatically)
 *   - Permissions: files.content.read, files.content.write
 *   - Redirect URIs: add the deployed URL of this page (e.g. https://you.github.io/doitdoit/)
 *
 * Then move your existing JSON file into the app folder and update the CLI's
 * ~/.doitdoit_config.json `storage_path` to match.
 */
window.DOITDOIT_CONFIG = {
  // Public app key (a.k.a. client ID). PKCE flow — no secret needed.
  dropboxAppKey: "",

  // Path inside the chosen Dropbox scope.
  // For an App-folder app, this is relative to /Apps/<your-app>/.
  // The leading "/" is required by the Dropbox API.
  dropboxFilePath: "/doitdoit.json",

  // Number of upcoming days rendered (today + next N-1).
  visibleDays: 5,

  // Mirrors model/task.go:14 — keep in sync with CLI.
  pruneAfterDays: 5,
};
