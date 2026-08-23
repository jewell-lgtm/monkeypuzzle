package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// renderStackStatus renders a StackStatusResult as the human tree shown on a
// terminal. JSON stays the contract for pipes/agents; this is presentation only.
//
//	main
//	├── ◆ api-rate-limit      in review  #6  +210 -18
//	│   └── ◆ api-rate-limit-cli  draft  #7  +62 -4
//	└── ◇ fix-flaky-retry     no PR          +5 -1
func renderStackStatus(result stackcmd.StackStatusResult, p cli.Painter) string {
	var rows []statusRow
	if result.Tree != nil {
		collectStatusRows(result.Tree.Children, "", &rows)
	}

	// Column widths come from the plain (uncolored) text, so ANSI codes never
	// skew the alignment.
	var nameW, statusW, numW, addsW int
	for _, r := range rows {
		if r.drift {
			continue
		}
		nameW = max(nameW, runeLen(r.lead)+2+runeLen(r.name))
		statusW = max(statusW, runeLen(r.status))
		numW = max(numW, runeLen(r.number))
		addsW = max(addsW, runeLen(r.adds))
	}

	var b strings.Builder
	b.WriteString(p.Paint(cli.SGRDim, result.MainBranch) + "\n")
	for _, r := range rows {
		if r.drift {
			b.WriteString(r.lead + p.Paint(cli.SGRYellow, cli.GlyphWarn) + " " + r.text + "\n")
			continue
		}
		line := r.lead + p.Paint(r.glyphCode, r.glyph) + " " + r.name +
			strings.Repeat(" ", nameW-(runeLen(r.lead)+2+runeLen(r.name))) + "  " +
			column(p, r.status, r.statusCode, statusW) + "  " +
			column(p, r.number, cli.SGRDim, numW) + "  " +
			column(p, r.adds, cli.SGRGreen, addsW) + " " +
			p.Paint(cli.SGRRed, r.dels)
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	if !result.ForgeChecked {
		b.WriteString("\n" + p.Paint(cli.SGRYellow, cli.GlyphWarn) + " forge unreachable — PR/MR state not checked\n")
	}
	if len(result.Reconstructed) > 0 {
		b.WriteString(fmt.Sprintf("\nreconstructed from forge: %s\n", strings.Join(result.Reconstructed, ", ")))
	}
	if len(result.Applied) > 0 {
		b.WriteString(fmt.Sprintf("bases updated on forge: %s\n", strings.Join(result.Applied, ", ")))
	}
	return b.String()
}

// statusRow is one output line: either a piece with its badge columns, or a
// drift warning attached beneath it.
type statusRow struct {
	drift bool
	lead  string // tree prefix incl. connector
	text  string // drift rows only

	glyph, glyphCode   string
	name               string
	status, statusCode string
	number             string // "#6", or "" when the piece has no PR
	adds, dels         string // "+62" / "-4", or "" without a diffstat
}

func collectStatusRows(nodes []*stackcmd.StackNode, prefix string, rows *[]statusRow) {
	for i, node := range nodes {
		connector, childPrefix := treeBranch(prefix, i == len(nodes)-1)
		row := statusRow{lead: prefix + connector, name: node.Piece}
		row.glyph, row.glyphCode, row.status, row.statusCode = nodeBadge(node.PR)
		if node.PR != nil {
			row.number = fmt.Sprintf("#%d", node.PR.Number)
		}
		if node.Diff != nil {
			row.adds = fmt.Sprintf("+%d", node.Diff.Additions)
			row.dels = fmt.Sprintf("-%d", node.Diff.Deletions)
		}
		*rows = append(*rows, row)
		for _, d := range node.Drift {
			*rows = append(*rows, statusRow{drift: true, lead: childPrefix, text: d})
		}
		collectStatusRows(node.Children, childPrefix, rows)
	}
}

// nodeBadge maps a piece's PR/MR state onto its glyph and status word.
func nodeBadge(pr *stackcmd.StackNodePR) (glyph, glyphCode, status, statusCode string) {
	if pr == nil {
		return "◇", cli.SGRDim, "no PR", cli.SGRDim
	}
	switch {
	case pr.State == "OPEN" && pr.Draft:
		return "◆", cli.SGRDim, "draft", cli.SGRDim
	case pr.State == "OPEN":
		return "◆", cli.SGRCyan, "in review", cli.SGRCyan
	case pr.State == "MERGED":
		return "●", cli.SGRGreen, "merged ✓", cli.SGRGreen
	case pr.State == "CLOSED":
		return "✗", cli.SGRRed, "closed", cli.SGRRed
	default:
		return "◆", "", strings.ToLower(pr.State), ""
	}
}

// column paints text and pads it (plain spaces) to the column width.
func column(p cli.Painter, text, code string, width int) string {
	return p.Paint(code, text) + strings.Repeat(" ", width-runeLen(text))
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// renderStackGraph renders a GraphResult (the no-clone forge forest) for a
// terminal, one tree per stack:
//
//	jewell-lgtm/mp-demo  (main, github)
//	└── #6 feat: add rate limiter  [api-rate-limit]
//	    └── #7 feat: wire rate limit into CLI  [api-rate-limit-cli]
func renderStackGraph(result stackcmd.GraphResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s  (%s, %s)\n", result.Repo, result.DefaultBranch, result.Provider))
	if len(result.Stacks) == 0 {
		b.WriteString("no open stacked PRs\n")
		return b.String()
	}
	for _, s := range result.Stacks {
		if s.Root == nil {
			continue
		}
		renderGraphNodes(&b, []*stackgraph.StackNode{s.Root}, "")
	}
	return b.String()
}

func renderGraphNodes(b *strings.Builder, nodes []*stackgraph.StackNode, prefix string) {
	for i, node := range nodes {
		connector, childPrefix := treeBranch(prefix, i == len(nodes)-1)
		draft := ""
		if node.PR.Draft {
			draft = " (draft)"
		}
		b.WriteString(fmt.Sprintf("%s%s#%d %s%s  [%s]\n", prefix, connector, node.PR.Number, node.PR.Title, draft, node.PR.HeadRef))
		renderGraphNodes(b, node.Children, childPrefix)
	}
}

// treeBranch returns the connector for a node and the prefix its children
// inherit, matching the box-drawing style of `mp list`.
func treeBranch(prefix string, last bool) (connector, childPrefix string) {
	if last {
		return "└── ", prefix + "    "
	}
	return "├── ", prefix + "│   "
}
