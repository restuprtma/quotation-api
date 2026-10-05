package dto

// StartMeetingExtractionRequest is the client-facing input for the resumable
// meeting extraction API. MeetingNotes is optional: the server normally reads
// the immutable notes from the quotation revision and only uses this value to
// verify a client still has the same input.
type StartMeetingExtractionRequest struct {
	MeetingNotes    string `json:"meeting_notes,omitempty"`
	ServiceScheme   string `json:"service_scheme"`
	PipelineVersion string `json:"pipeline_version"`
	IdempotencyKey  string `json:"idempotency_key,omitempty"`
}

type RetryMeetingExtractionRequest struct {
	NoteHash string `json:"note_hash,omitempty"`
}

type MeetingExtractionStartResponse struct {
	RunID       string `json:"run_id"`
	QuotationID string `json:"quotation_id"`
	Status      string `json:"status"`
	StatusURL   string `json:"status_url"`
}

type MeetingExtractionProgress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type MeetingExtractionPhaseView struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Status    string  `json:"status"`
	Attempt   int     `json:"attempt"`
	ErrorCode *string `json:"error_code"`
	Retryable bool    `json:"retryable"`
}

type MeetingExtractionBudgetView struct {
	CallsUsed int `json:"calls_used"`
	CallsMax  int `json:"calls_max"`
}

type MeetingExtractionNotice struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type MeetingExtractionErrorView struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RetryAfter *int64 `json:"retry_after,omitempty"`
}

// MeetingExtractionStatusResponse is deliberately a safe projection. It has
// no notes, result JSON, quote/content, prompt, provider response, token,
// lease, or fence fields.
type MeetingExtractionStatusResponse struct {
	RunID        string                       `json:"run_id"`
	QuotationID  string                       `json:"quotation_id"`
	Status       string                       `json:"status"`
	CurrentPhase *string                      `json:"current_phase"`
	Progress     MeetingExtractionProgress    `json:"progress"`
	Phases       []MeetingExtractionPhaseView `json:"phases"`
	FailedPhase  *string                      `json:"failed_phase"`
	Error        *MeetingExtractionErrorView  `json:"error"`
	Budget       MeetingExtractionBudgetView  `json:"budget"`
	NeedsInput   []MeetingExtractionNotice    `json:"needs_input"`
	Warnings     []MeetingExtractionNotice    `json:"warnings"`
}

type MeetingExtractionErrorResponse struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RetryAfter *int64 `json:"retry_after,omitempty"`
}
