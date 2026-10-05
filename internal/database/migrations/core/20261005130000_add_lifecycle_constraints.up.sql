CREATE UNIQUE INDEX IF NOT EXISTS idx_quotation_revisions_single_draft 
ON quotation_revisions(quotation_id) 
WHERE publication_status = 'draft';

CREATE UNIQUE INDEX IF NOT EXISTS idx_blueprint_revisions_single_draft 
ON blueprint_revisions(blueprint_id) 
WHERE publication_status = 'draft';
