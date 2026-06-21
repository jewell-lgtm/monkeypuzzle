package web

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// Views are type-safe HTML built with gomponents: the Go compiler checks every
// field access, so a renamed store/stackgraph field is a build error rather than
// a silently-blank template variable.

// page wraps body content in a full HTML document; the head loads the stylesheet
// and AlpineJS.
func page(title string, body ...g.Node) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    title,
		Language: "en",
		Head: g.Group{
			Link(Rel("stylesheet"), Href("/static/app.css")),
			Script(Src("/static/alpine.min.js"), Defer()),
		},
		Body: g.Group(body),
	})
}

func navBar() g.Node {
	return Header(Class("nav"),
		A(Class("brand"), Href("/"), g.Text("🐒 Monkey Puzzle")),
		Form(Method("post"), Action("/logout"),
			Button(Class("link"), Type("submit"), g.Text("Sign out")),
		),
	)
}

// dashboardPage renders the shell instantly; AlpineJS hydrates the repo list
// from /partials/repos (fast TTFB) and polls while a first sync runs.
func dashboardPage() g.Node {
	return page("Monkey Puzzle",
		navBar(),
		Main(g.Attr("x-data", "dashboard()"), g.Attr("x-init", "init()"),
			Div(Class("bar"),
				H1(g.Text("Your repositories")),
				Button(
					g.Attr("@click", "sync()"),
					g.Attr(":disabled", "syncing"),
					g.Attr("x-text", "syncing ? 'Syncing…' : 'Sync now'"),
					g.Text("Sync now"),
				),
			),
			Div(g.Attr("x-show", "loading"), Class("skeleton"),
				Div(Class("skeleton-row")), Div(Class("skeleton-row")), Div(Class("skeleton-row")),
			),
			Div(g.Attr("x-show", "!loading"), g.Attr("x-html", "reposHtml")),
		),
		Script(g.Raw(dashboardJS)),
	)
}

const dashboardJS = `function dashboard() {
  return {
    loading: true, syncing: false, reposHtml: '',
    async init() { await this.load(); this.maybePoll(); },
    async load() {
      this.loading = true;
      const r = await fetch('/partials/repos');
      this.reposHtml = await r.text();
      this.loading = false;
    },
    async maybePoll() {
      const s = await (await fetch('/sync/status')).json();
      if (s.status === 'running') {
        setTimeout(async () => { await this.load(); this.maybePoll(); }, 1500);
      }
    },
    async sync() {
      this.syncing = true;
      await fetch('/sync', { method: 'POST' });
      setTimeout(async () => { this.syncing = false; await this.load(); this.maybePoll(); }, 800);
    },
  };
}`

// reposFragment is the hydrated repo-list fragment served to AlpineJS.
func reposFragment(repos []store.Repo, syncing bool) g.Node {
	switch {
	case len(repos) > 0:
		return Ul(Class("repos"), g.Map(repos, func(r store.Repo) g.Node {
			return Li(
				A(Href("/repos/"+r.Owner+"/"+r.Name), g.Text(r.Owner+"/"+r.Name)),
				g.If(r.Private, Span(Class("tag"), g.Text("private"))),
				Span(Class("branch"), g.Text(r.DefaultBranch)),
			)
		}))
	case syncing:
		return P(Class("muted"), g.Text("Syncing your repositories from GitHub…"))
	default:
		return P(Class("muted"), g.Text("No repositories yet. Try Sync now."))
	}
}

// repoPageView renders a repo's PR stacks as nested trees.
func repoPageView(repo store.Repo, stacks []stackgraph.Stack) g.Node {
	var content g.Node
	if len(stacks) == 0 {
		content = P(Class("muted"), g.Text("No open pull-request stacks."))
	} else {
		content = g.Map(stacks, func(s stackgraph.Stack) g.Node {
			return Section(Class("stack"), Ol(Class("stack-tree"), stackNode(s.Root)))
		})
	}
	return page(repo.Owner+"/"+repo.Name,
		navBar(),
		Main(
			P(A(Href("/"), g.Text("← all repositories"))),
			H1(g.Text(repo.Owner+"/"+repo.Name)),
			content,
		),
	)
}

// stackNode renders one PR and, recursively, the PRs stacked on it.
func stackNode(n *stackgraph.StackNode) g.Node {
	hasChildren := len(n.Children) > 0
	var toggle g.Node
	if hasChildren {
		toggle = Button(Class("toggle"), g.Attr("@click", "open = !open"), g.Attr("x-text", "open ? '▾' : '▸'"), g.Text("▾"))
	} else {
		toggle = Span(Class("toggle-spacer"))
	}
	return Li(g.Attr("x-data", "{ open: true }"),
		Div(Class("pr"),
			toggle,
			A(Class("pr-num"), Href(n.PR.URL), Target("_blank"), Rel("noopener"), g.Textf("#%d", n.PR.Number)),
			Span(Class("pr-title"), g.Text(n.PR.Title)),
			Span(Class("state state-"+n.PR.State), g.Text(n.PR.State)),
			g.If(n.PR.Draft, Span(Class("tag"), g.Text("draft"))),
			Span(Class("muted"), g.Text(n.PR.Author)),
		),
		g.If(hasChildren, Ol(g.Attr("x-show", "open"), g.Map(n.Children, stackNode))),
	)
}
