-- Phase 11: Optimized lookup index for public quotation token resolution and signature
CREATE INDEX IF NOT EXISTS idx_quotations_public_token_active 
ON quotations(public_token) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_quotation_revisions_lookup_published
ON quotation_revisions(id, quotation_id, publication_status);
