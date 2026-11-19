// Package sourcetype says what kind of publisher a source is, so readers can weigh a story:
// an official body, a think tank, a news outlet, an analysis magazine, a company's engineering
// blog or an open-source project's blog. The map is keyed by host and must name every host in
// sources.json (a test enforces it).
package sourcetype

const (
	Official  = "official"
	ThinkTank = "think-tank"
	News      = "news"
	Analysis  = "analysis"
	Company   = "company"
	Project   = "project"
)

var byHost = map[string]string{
	// India, world and UPSC
	"news.un.org":            Official,
	"www.orfonline.org":      ThinkTank,
	"indianexpress.com":      News,
	"www.thehindu.com":       News,
	"entrackr.com":           News,
	"inc42.com":              News,
	"techcrunch.com":         News,
	"news.crunchbase.com":    Analysis,
	"www.livemint.com":       News,
	"scroll.in":              News,
	"www.aljazeera.com":      News,
	"www.theguardian.com":    News,
	"www.downtoearth.org.in": News,
	"foreignpolicy.com":      Analysis,
	"thediplomat.com":        Analysis,
	"restofworld.org":        Analysis,
	// technology and AI
	"www.infoq.com":              News,
	"blog.bytebytego.com":        Analysis,
	"css-tricks.com":             Analysis,
	"martinfowler.com":           Analysis,
	"www.latent.space":           Analysis,
	"www.smashingmagazine.com":   Analysis,
	"blog.rust-lang.org":         Project,
	"go.dev":                     Project,
	"kubernetes.io":              Project,
	"react.dev":                  Project,
	"inside.java":                Project,
	"www.cncf.io":                Project,
	"aws.amazon.com":             Company,
	"blog.cloudflare.com":        Company,
	"blogs.cisco.com":            Company,
	"cloud.google.com":           Company,
	"developer.nvidia.com":       Company,
	"developers.googleblog.com":  Company,
	"developers.redhat.com":      Company,
	"engineering.atspotify.com":  Company,
	"engineering.fb.com":         Company,
	"engineering.grab.com":       Company,
	"engineering.salesforce.com": Company,
	"engineering.zalando.com":    Company,
	"github.blog":                Company,
	"huggingface.co":             Company,
	"innovation.ebayinc.com":     Company,
	"shopify.engineering":        Company,
	"snyk.io":                    Company,
	"stripe.dev":                 Company,
	"supabase.com":               Company,
	"www.amazon.science":         Company,
	"www.anthropic.com":          Company,
	"www.canva.dev":              Company,
	"www.datadoghq.com":          Company,
	"www.heroku.com":             Company,
	"www.honeycomb.io":           Company,
	"www.netlify.com":            Company,
	"www.uber.com":               Company,
}

// For returns the type of the source with this host, or "" when it is not known.
func For(host string) string { return byHost[host] }
