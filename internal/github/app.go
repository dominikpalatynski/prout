package github

import (
	"net/http"
)

func (gh *GithubClient) LoadGithubSetupPage(w http.ResponseWriter, r *http.Request) {
	//     configured := s.githubConfigExists()

	//     w.Header().Set("Content-Type", "text/html; charset=utf-8")

	//     if configured {
	//         _, _ = w.Write([]byte(`
	// <!doctype html>
	// <html>
	//   <head>
	//     <title>GitHub Setup</title>
	//   </head>
	//   <body>
	//     <h1>GitHub Setup</h1>
	//     <p><strong>Status:</strong> configured</p>

	//     <hr />

	//     <h2>Danger zone</h2>
	//     <form method="post" action="/settings/github-setup/reset">
	//       <label>Admin secret:</label><br />
	//       <input type="password" name="admin_secret" style="width: 400px;" /><br /><br />

	//       <label>Type RESET to confirm:</label><br />
	//       <input type="text" name="confirm" /><br /><br />

	//       <button type="submit">Reset GitHub integration</button>
	//     </form>
	//   </body>
	// </html>
	// `))
	//         return
	//     }

	_, _ = w.Write([]byte(`
<!doctype html>
<html>
  <head>
    <title>GitHub Setup</title>
  </head>
  <body>
    <h1>GitHub Setup</h1>
    <p><strong>Status:</strong> not configured</p>

    <form method="post" action="/settings/github-setup/start">
      <label>Admin secret:</label><br />
      <input type="password" name="admin_secret" style="width: 400px;" /><br /><br />

      <button type="submit">Setup GitHub</button>
    </form>
  </body>
</html>
`))
}
