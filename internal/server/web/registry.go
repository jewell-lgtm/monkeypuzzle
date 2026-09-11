package web

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html" //nolint:staticcheck // gomponents views use dot imports throughout this package
)

func registryPage(items []tracking.Item, prEnabled bool, query url.Values) g.Node {
	projects, machines := map[string]string{}, map[string]string{}
	visible := []tracking.Item{}
	for _, item := range items {
		projects[item.ProjectID], machines[item.MachineID] = item.Project, item.Machine
		if id := query.Get("project"); id != "" && item.ProjectID != id {
			continue
		}
		if id := query.Get("machine"); id != "" && item.MachineID != id {
			continue
		}
		if state := query.Get("state"); state != "" && item.State != state {
			continue
		}
		if search := strings.ToLower(query.Get("q")); search != "" && !strings.Contains(strings.ToLower(item.Piece+" "+item.Project+" "+item.Machine+" "+item.Note), search) {
			continue
		}
		visible = append(visible, item)
	}
	states := map[string]string{"todo": "To do", "working": "Working", "blocked": "Blocked", "review": "Review", "done": "Done"}
	return page("Piece registry · Monkeypuzzle", navBar(),
		Main(Class("registry"),
			Div(Class("bar"), H1(g.Text("Your pieces")), A(Href("/"), g.Text("Refresh"))),
			P(Class("muted"), g.Text("Your private registry across machines and projects. Only pieces you explicitly publish appear here.")),
			Div(Class("registry-counts"),
				Span(g.Textf("%d pieces", len(items))), Span(g.Textf("%d projects", len(projects))), Span(g.Textf("%d machines", len(machines)))),
			Form(Method("get"), Action("/"), Class("registry-filters"),
				Label(g.Text("Search"), Input(Name("q"), Type("search"), Value(query.Get("q")), Placeholder("Piece or note"))),
				registrySelect("project", "Project", projects, query.Get("project")),
				registrySelect("machine", "Machine", machines, query.Get("machine")),
				registrySelect("state", "State", states, query.Get("state")),
				Button(Type("submit"), g.Text("Filter")),
				A(Href("/"), g.Text("Clear"))),
			g.If(len(items) == 0, Section(Class("registry-empty"),
				H2(g.Text("Publish your first piece")),
				P(g.Text("From a piece on any of your machines, configure your server URL and access token, then run:")),
				Pre(Code(g.Text("mp tracking report --state working --note 'Getting started'"))),
				P(Class("muted"), g.Text("Your usual mp workflow stays local. Publishing is always explicit.")))),
			g.If(len(items) > 0 && len(visible) == 0, P(g.Text("No pieces match these filters."))),
			g.If(len(visible) > 0, Div(Class("registry-table-wrap"), Table(Class("registry-table"),
				THead(Tr(Th(g.Text("Piece / project")), Th(g.Text("Machine")), Th(g.Text("Progress")), Th(g.Text("Last reported change")))),
				TBody(g.Map(visible, func(item tracking.Item) g.Node {
					return Tr(
						Td(Strong(g.Text(item.Piece)), Div(Class("muted"), g.Text(item.Project)),
							g.If(item.Note != "", P(Class("piece-note"), g.Text(item.Note))),
							Details(Summary(g.Text("Location and identity")),
								g.If(item.WorktreePath != "", P(Code(g.Text(item.WorktreePath)))),
								g.If(item.Parent != "", P(g.Text("Parent: "+item.Parent))),
								P(Class("identity"), g.Text("Machine: "+item.MachineID), Br(), g.Text("Project: "+item.ProjectID), Br(), g.Text("Piece: "+item.PieceID)))),
						Td(g.Text(item.Machine)),
						Td(Span(Class("state registry-state-"+item.State), g.Text(item.State))),
						Td(Time(DateTime(item.UpdatedAt.Format(time.RFC3339)), g.Text(item.UpdatedAt.UTC().Format("02 Jan 2006, 15:04 UTC")))),
					)
				}))))),
			g.If(len(items) > 0, P(Class("muted"), g.Text("Progress reflects the last explicit report, not live activity. Marking a piece done keeps it here; mp settle removes it from the registry."))),
			g.If(prEnabled, P(A(Href("/repositories"), g.Text("PR monitoring →")))),
		),
	)
}

func registrySelect(name, label string, choices map[string]string, selected string) g.Node {
	keys := make([]string, 0, len(choices))
	for key := range choices {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if choices[keys[i]] == choices[keys[j]] {
			return keys[i] < keys[j]
		}
		return choices[keys[i]] < choices[keys[j]]
	})
	return Label(g.Text(label), Select(Name(name),
		Option(Value(""), g.Text("All")),
		g.Map(keys, func(key string) g.Node {
			return Option(Value(key), g.If(key == selected, Selected()), g.Text(choices[key]))
		})))
}
