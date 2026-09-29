package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergePromptSection_ReplacesOnlyManagedTargetAndPreservesOtherSections(t *testing.T) {
	current := `<article class="quotation-document"><section data-quotation-section="scope" data-section-version="1"><table class="manual" data-x="kept"><tbody><tr><td>BA edit</td></tr></tbody></table></section><section data-managed="true" data-section-version="1" data-quotation-section="commercial"><p>old commercial</p></section><section data-quotation-section="payment" data-section-version="1" data-managed="true"><p>old payment</p></section><section data-quotation-section="legal"><p>legal copy</p></section></article>`
	replacement := `<section data-quotation-section="commercial" data-section-version="1" data-managed="true"><table><tbody><tr><td>new commercial</td></tr></tbody></table></section>`

	merged, err := MergePromptSection(current, "commercial", replacement)
	require.NoError(t, err)
	require.Contains(t, merged, `<section data-quotation-section="scope" data-section-version="1"><table class="manual" data-x="kept"><tbody><tr><td>BA edit</td></tr></tbody></table></section>`)
	require.Contains(t, merged, `<section data-quotation-section="legal"><p>legal copy</p></section>`)
	require.Contains(t, merged, `new commercial`)
	require.NotContains(t, merged, `old commercial`)
}

func TestMergePromptSection_RejectsMissingOrDuplicateTarget(t *testing.T) {
	replacement := `<section data-quotation-section="commercial" data-section-version="1" data-managed="true"><p>new</p></section>`
	_, err := MergePromptSection(`<article><section data-quotation-section="scope">scope</section></article>`, "commercial", replacement)
	require.ErrorContains(t, err, "exactly one")

	_, err = MergePromptSection(`<article><section data-quotation-section="commercial"></section><section data-quotation-section="commercial"></section></article>`, "commercial", replacement)
	require.ErrorContains(t, err, "exactly one")
}
