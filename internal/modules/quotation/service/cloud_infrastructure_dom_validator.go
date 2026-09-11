package service

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type CloudInfrastructureIntegrityExpectations struct {
	Input       domain.CloudInfrastructureInput
	Calculation domain.CloudInfrastructureCalculation
	Profile     domain.ResolvedCloudInfrastructureProfile
	LegalHTML   string
}

func ValidateCloudInfrastructureIntegrity(documentHTML string, expected CloudInfrastructureIntegrityExpectations) error {
	in := expected.Input.Normalize()
	if err := in.Validate(); err != nil {
		return fmt.Errorf("cloud input: %w", err)
	}
	calc := expected.Calculation
	if len(calc.Packages) == 0 {
		var err error
		calc, err = CalculateCloudInfrastructure(in)
		if err != nil {
			return err
		}
	}
	profile := expected.Profile
	if len(profile.Sections) == 0 {
		var err error
		profile, err = ResolveCloudInfrastructureProfile(in)
		if err != nil {
			return err
		}
	}
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return fmt.Errorf("document parse: %w", err)
	}
	nodes := cloudNodes(root)
	article := cloudExactly(nodes, "class", "quotation-document quotation-document--cloud-infrastructure")
	if article == nil {
		return fmt.Errorf("cloud document root is missing")
	}
	for _, sec := range profile.Sections {
		n := cloudExactly(nodes, "data-section-code", sec.Code)
		if n == nil {
			return fmt.Errorf("section %s is missing", sec.Code)
		}
		if cloudAttr(n, "data-section-number") != fmt.Sprint(sec.Number) {
			return fmt.Errorf("section %s number mismatch", sec.Code)
		}
	}
	if cloudExactly(nodes, "data-block-type", "signature") == nil {
		return fmt.Errorf("signature block is missing")
	}
	if err := cloudCheckCommercial(root, in, calc, profile); err != nil {
		return err
	}
	if profile.ShowScope {
		if err := cloudCheckScope(root, in); err != nil {
			return err
		}
	} else if cloudExactly(nodes, "data-section-code", "scope_timeline") != nil {
		return fmt.Errorf("scope section is unexpected")
	}
	if modified, err := CheckCloudInfrastructureLimitationsModified(documentHTML, in.LimitationCodes, in.AdditionalLimitations); err != nil {
		return err
	} else if modified {
		return fmt.Errorf("limitations content was modified")
	}
	if strings.TrimSpace(expected.LegalHTML) != "" {
		if cloudCountValue(nodes, "data-block-type", "legal") < 2 {
			return fmt.Errorf("legal block is missing")
		}
	}
	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}
	return nil
}

func CheckCloudInfrastructureCommercialModified(documentHTML string, input domain.CloudInfrastructureInput, calc domain.CloudInfrastructureCalculation) (bool, error) {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, err
	}
	p, err := ResolveCloudInfrastructureProfile(input)
	if err != nil {
		return true, err
	}
	return cloudCheckCommercial(root, input.Normalize(), calc, p) != nil, nil
}
func cloudCheckCommercial(root *html.Node, in domain.CloudInfrastructureInput, calc domain.CloudInfrastructureCalculation, p domain.ResolvedCloudInfrastructureProfile) error {
	nodes := cloudNodes(root)
	cost := cloudExactly(nodes, "data-section-code", "cloud_cost")
	if cost == nil {
		return fmt.Errorf("cloud_cost section is missing")
	}
	cn := cloudNodes(cost)
	rows := cloudAll(cn, "data-cloud-package-row")
	if len(rows) != len(calc.Packages) {
		return fmt.Errorf("package row count mismatch: got %d want %d", len(rows), len(calc.Packages))
	}
	for i, x := range calc.Packages {
		r := rows[i]
		if cloudAttr(r, "data-cloud-package-row") != fmt.Sprint(i) {
			return fmt.Errorf("package row %d index mismatch", i)
		}
		cells := cloudDirect(r, "td")
		if len(cells) != 6 {
			return fmt.Errorf("package row %d has invalid column count", i)
		}
		want := []string{x.Name, fmt.Sprintf("%d vCPU", x.VCPU), fmt.Sprintf("%d GB", x.RAMGB), fmt.Sprintf("%d GB", x.StorageGB), cloudMoney(x.UnitRate), cloudMoney(x.LineTotal)}
		for j := range want {
			if strings.TrimSpace(cloudText(cells[j])) != want[j] {
				return fmt.Errorf("package row %d field %d mismatch", i, j)
			}
		}
	}
	total := cloudExactly(cn, "data-cloud-field", "monthly_subtotal")
	if total == nil || strings.TrimSpace(cloudText(total)) != cloudMoney(calc.MonthlySubtotal) {
		return fmt.Errorf("monthly_subtotal mismatch")
	}
	setup := cloudAll(cn, "data-cloud-setup-row")
	if p.ShowSetup {
		if len(setup) != len(calc.Setup) {
			return fmt.Errorf("setup row count mismatch")
		}
	} else if len(setup) > 0 || cloudExactly(cn, "data-cloud-field", "setup_subtotal") != nil {
		return fmt.Errorf("unexpected setup block")
	}
	pay := cloudExactly(nodes, "data-section-code", "payment")
	if pay == nil {
		return fmt.Errorf("payment section is missing")
	}
	pr := cloudAll(cloudNodes(pay), "data-cloud-payment-row")
	if len(pr) != len(in.PaymentSchedule) {
		return fmt.Errorf("payment row count mismatch")
	}
	for i, x := range in.PaymentSchedule {
		cells := cloudDirect(pr[i], "td")
		if len(cells) != 5 {
			return fmt.Errorf("payment row %d invalid column count", i)
		}
		want := []string{x.WorkPeriod, x.Terms, x.InvoiceDateLabel, x.DueDateLabel, cloudMoney(x.Nominal)}
		for j := range want {
			if strings.TrimSpace(cloudText(cells[j])) != want[j] {
				return fmt.Errorf("payment row %d field %d mismatch", i, j)
			}
		}
		if cloudAttr(pr[i], "data-cloud-payment-component") != string(x.Component) {
			return fmt.Errorf("payment row %d component mismatch", i)
		}
	}
	return nil
}
func cloudCheckScope(root *html.Node, in domain.CloudInfrastructureInput) error {
	sec := cloudExactly(cloudNodes(root), "data-section-code", "scope_timeline")
	if sec == nil {
		return fmt.Errorf("scope section is missing")
	}
	rows := cloudAll(cloudNodes(sec), "data-cloud-scope-row")
	if len(rows) != len(in.ScopeRows) {
		return fmt.Errorf("scope row count mismatch")
	}
	for i, row := range in.ScopeRows {
		text := cloudText(rows[i])
		for _, s := range row.Sections {
			if !strings.Contains(text, s.Title) {
				return fmt.Errorf("scope subsection %q is missing", s.Title)
			}
			for _, a := range s.Activities {
				if !strings.Contains(text, a) {
					return fmt.Errorf("scope activity %q is missing", a)
				}
			}
		}
		timeline := cloudExactly(cloudNodes(rows[i]), "data-cloud-scope-field", "timeline")
		if timeline == nil || strings.TrimSpace(cloudText(timeline)) != row.Timeline {
			return fmt.Errorf("scope row %d timeline mismatch", i)
		}
	}
	return nil
}
func CheckCloudInfrastructureLimitationsModified(documentHTML string, codes []domain.CloudLimitationCode, additional []string) (bool, error) {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, err
	}
	sec := cloudExactly(cloudNodes(root), "data-section-code", "limitations")
	if sec == nil {
		return true, nil
	}
	lis := cloudDirect(cloudExactly(cloudNodes(sec), "data", "impossible"), "li")
	_ = lis
	expected := cloudLimitationTexts(domain.CloudInfrastructureInput{LimitationCodes: codes, AdditionalLimitations: additional})
	got := []string{}
	for _, n := range cloudNodes(sec) {
		if n.Type == html.ElementNode && n.Data == "li" {
			got = append(got, strings.TrimSpace(cloudText(n)))
		}
	}
	if len(got) != len(expected) {
		return true, nil
	}
	for i := range got {
		if got[i] != strings.TrimSpace(expected[i]) {
			return true, nil
		}
	}
	return false, nil
}
func ResolveCloudInfrastructureLegalHTML(base string, _ domain.CloudTaxTreatment) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", nil
	}
	root, err := html.Parse(strings.NewReader("<div>" + base + "</div>"))
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	body := root
	for _, n := range cloudNodes(root) {
		if n.Type == html.ElementNode && n.Data == "div" {
			body = n
			break
		}
	}
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}
func cloudNodes(root *html.Node) []*html.Node {
	out := []*html.Node{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		out = append(out, n)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}
func cloudAttr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func cloudExactly(nodes []*html.Node, key, value string) *html.Node {
	var out *html.Node
	for _, n := range nodes {
		match := false
		if key == "class" {
			match = cloudAttr(n, key) == value
		} else {
			match = cloudAttr(n, key) == value
		}
		if match {
			if out != nil {
				return nil
			}
			out = n
		}
	}
	return out
}
func cloudAll(nodes []*html.Node, key string) []*html.Node {
	out := []*html.Node{}
	for _, n := range nodes {
		if cloudAttr(n, key) != "" {
			out = append(out, n)
		}
	}
	return out
}
func cloudCountValue(nodes []*html.Node, key, value string) int {
	count := 0
	for _, n := range nodes {
		if cloudAttr(n, key) == value {
			count++
		}
	}
	return count
}
func cloudDirect(n *html.Node, tag string) []*html.Node {
	if n == nil {
		return nil
	}
	out := []*html.Node{}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			out = append(out, c)
		}
	}
	return out
}
func cloudText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
