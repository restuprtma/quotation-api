package rnd

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestBlueprintTypesRoundTrip(t *testing.T) {
	sampleBundleJSON := `{
		"schema_version": "1.0",
		"title": "Sistem Manajemen Bengkel Terintegrasi",
		"summary": "Platform terpadu untuk reservasi servis, pelacakan pengerjaan, dan inventaris bengkel.",
		"journey": {
			"phases": [
				{
					"id": "phase-booking",
					"label": "Reservasi & Kedatangan",
					"order": 1
				},
				{
					"id": "phase-servicing",
					"label": "Pengerjaan Servis",
					"order": 2
				}
			],
			"actors": [
				{
					"id": "actor-customer",
					"label": "Pelanggan",
					"order": 1
				},
				{
					"id": "actor-mechanic",
					"label": "Mekanik",
					"order": 2
				}
			],
			"activities": [
				{
					"id": "act-create-booking",
					"phase_id": "phase-booking",
					"actor_id": "actor-customer",
					"label": "Buat Janji Servis",
					"description": "Pelanggan memilih jadwal servis melalui aplikasi.",
					"touchpoints": [
						"Mobile App",
						"WhatsApp Notification"
					],
					"references": [
						"architecture.node.node-booking-api",
						"state.state-booked",
						"workflow.wf-booking.step.wnode-submit-booking",
						"wbs.module.mod-booking"
					]
				}
			]
		},
		"architecture": {
			"groups": [
				{
					"id": "grp-frontend",
					"label": "Frontend Tier",
					"order": 1
				},
				{
					"id": "grp-backend",
					"label": "Backend Tier",
					"order": 2
				}
			],
			"nodes": [
				{
					"id": "node-booking-api",
					"group_id": "grp-backend",
					"type": "service",
					"label": "Booking Service",
					"description": "Layanan pemesanan dan jadwal servis.",
					"layout": {
						"x": 250.5,
						"y": 120.0
					}
				}
			],
			"edges": [
				{
					"id": "edge-client-api",
					"source": "node-booking-api",
					"target": "node-booking-api",
					"label": "gRPC / JSON",
					"type": "sync"
				}
			]
		},
		"state_machine": {
			"entity": "ServiceOrder",
			"initial_state": "state-booked",
			"states": [
				{
					"id": "state-booked",
					"label": "Booked",
					"type": "normal",
					"layout": {
						"x": 100.0,
						"y": 100.0
					}
				},
				{
					"id": "state-completed",
					"label": "Completed",
					"type": "terminal",
					"layout": {
						"x": 400.0,
						"y": 100.0
					}
				}
			],
			"transitions": [
				{
					"id": "trans-complete",
					"source": "state-booked",
					"target": "state-completed",
					"event": "FINISH_WORK",
					"actor": "actor-mechanic",
					"guard": "all_inspections_passed",
					"actions": [
						"notify_customer",
						"generate_invoice"
					]
				}
			]
		},
		"workflows": [
			{
				"id": "wf-booking",
				"name": "Alur Reservasi Servis",
				"description": "Proses bisnis pemesanan servis dari awal hingga konfirmasi.",
				"lanes": [
					{
						"id": "lane-customer",
						"label": "Pelanggan",
						"order": 1
					}
				],
				"nodes": [
					{
						"id": "wnode-submit-booking",
						"lane_id": "lane-customer",
						"type": "action",
						"label": "Kirim Reservasi",
						"description": "Pelanggan mengisi form dan konfirmasi jadwal.",
						"layout": {
							"x": 150.0,
							"y": 80.0
						}
					}
				],
				"edges": [
					{
						"id": "wedge-1",
						"source": "wnode-submit-booking",
						"target": "wnode-submit-booking",
						"label": "Lanjut"
					}
				]
			}
		],
		"wbs": {
			"modules": [
				{
					"id": "mod-booking",
					"name": "Manajemen Reservasi",
					"description": "Modul pengelolaan booking dan jadwal kedatangan.",
					"priority": "high",
					"dependencies": [],
					"references": [
						"architecture.node.node-booking-api"
					],
					"features": [
						{
							"id": "feat-slot-picker",
							"name": "Pemilihan Slot Servis",
							"description": "Fitur kalender slot bengkel interaktif.",
							"acceptance_criteria": [
								"Slot terupdate realtime",
								"Maksimal 5 booking per jam"
							]
						}
					]
				}
			]
		},
		"assumptions": [
			"Bengkel beroperasi 6 hari seminggu (Senin - Sabtu).",
			"Integrasi WhatsApp menggunakan gateway penyedia eksternal."
		],
		"open_questions": [
			{
				"id": "oq-payment-gateway",
				"question": "Apakah pembayaran uang muka (DP) diwajibkan saat booking?",
				"target_section": "workflow",
				"impact": "Mempengaruhi rancangan alur pembayaran dan integrasi payment gateway.",
				"status": "open"
			}
		],
		"cross_references": [
			{
				"id": "xref-1",
				"source_ref": "journey.activity.act-create-booking",
				"target_ref": "workflow.wf-booking.step.wnode-submit-booking",
				"relation_type": "implements",
				"description": "Aktivitas pemesanan diimplementasikan oleh step wnode-submit-booking pada workflow."
			}
		]
	}`

	var bundle BlueprintBundle
	if err := json.Unmarshal([]byte(sampleBundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal valid bundle JSON: %v", err)
	}

	if bundle.SchemaVersion != "1.0" {
		t.Errorf("expected SchemaVersion '1.0', got %q", bundle.SchemaVersion)
	}
	if bundle.Title != "Sistem Manajemen Bengkel Terintegrasi" {
		t.Errorf("expected Title 'Sistem Manajemen Bengkel Terintegrasi', got %q", bundle.Title)
	}
	if len(bundle.Journey.Phases) != 2 {
		t.Errorf("expected 2 journey phases, got %d", len(bundle.Journey.Phases))
	}
	if len(bundle.Journey.Actors) != 2 {
		t.Errorf("expected 2 journey actors, got %d", len(bundle.Journey.Actors))
	}
	if len(bundle.Journey.Activities) != 1 {
		t.Errorf("expected 1 journey activity, got %d", len(bundle.Journey.Activities))
	}
	if len(bundle.Architecture.Groups) != 2 {
		t.Errorf("expected 2 architecture groups, got %d", len(bundle.Architecture.Groups))
	}
	if len(bundle.Architecture.Nodes) != 1 {
		t.Errorf("expected 1 architecture node, got %d", len(bundle.Architecture.Nodes))
	}
	if bundle.Architecture.Nodes[0].Layout.X != 250.5 {
		t.Errorf("expected architecture node layout X 250.5, got %v", bundle.Architecture.Nodes[0].Layout.X)
	}
	if bundle.StateMachine.Entity != "ServiceOrder" {
		t.Errorf("expected StateMachine entity 'ServiceOrder', got %q", bundle.StateMachine.Entity)
	}
	if len(bundle.Workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(bundle.Workflows))
	}
	if len(bundle.WBS.Modules) != 1 {
		t.Errorf("expected 1 WBS module, got %d", len(bundle.WBS.Modules))
	}
	if len(bundle.Assumptions) != 2 {
		t.Errorf("expected 2 assumptions, got %d", len(bundle.Assumptions))
	}
	if len(bundle.OpenQuestions) != 1 {
		t.Errorf("expected 1 open question, got %d", len(bundle.OpenQuestions))
	}
	if len(bundle.CrossReferences) != 1 {
		t.Errorf("expected 1 cross reference, got %d", len(bundle.CrossReferences))
	}

	marshaled, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}

	var roundTripBundle BlueprintBundle
	if err := json.Unmarshal(marshaled, &roundTripBundle); err != nil {
		t.Fatalf("failed to unmarshal marshaled bundle: %v", err)
	}

	if !reflect.DeepEqual(bundle, roundTripBundle) {
		t.Errorf("round trip bundle does not match original bundle")
	}
}

func TestBlueprintInputRoundTrip(t *testing.T) {
	sampleInputJSON := `{
		"source_quotation_revision_id": "rev-1002-v1",
		"source_context": {
			"subject": "Pengembangan Sistem Reservasi",
			"solution_type": "custom_software",
			"summary": "Platform pemesanan janji temu klinik.",
			"scope": [
				"Modul Registrasi Pasien",
				"Modul Reservasi Dokter"
			],
			"timeline": "3 bulan"
		},
		"business_goals": [
			"Mengurangi waktu antrean",
			"Otomatisasi jadwal dokter"
		],
		"actors": [
			"Pasien",
			"Dokter",
			"Resepsionis"
		],
		"channels": [
			"Web Portal Pasien",
			"Admin Dashboard",
			"WhatsApp Gateway"
		],
		"business_phases": [
			"Registrasi",
			"Pemesanan",
			"Kunjungan",
			"Selesai"
		],
		"business_rules": [
			"Pembatalan maksimal 2 jam sebelum jadwal",
			"Dokter tidak dapat dibooking jika jadwal cuti"
		],
		"known_states": [
			"DRAFT",
			"CONFIRMED",
			"COMPLETED",
			"CANCELLED"
		],
		"integrations": [
			"Payment Gateway",
			"WhatsApp API"
		],
		"technology_preferences": [
			"Golang",
			"PostgreSQL",
			"React"
		],
		"operational_constraints": [
			"Dukungan koneksi lambat di mobile",
			"Audit trail untuk setiap perubahan reservasi"
		],
		"additional_notes": "Prioritaskan modul reservasi pada sprint awal."
	}`

	var input BlueprintInput
	if err := json.Unmarshal([]byte(sampleInputJSON), &input); err != nil {
		t.Fatalf("failed to unmarshal valid input JSON: %v", err)
	}

	if input.SourceQuotationRevisionID != "rev-1002-v1" {
		t.Errorf("expected SourceQuotationRevisionID 'rev-1002-v1', got %q", input.SourceQuotationRevisionID)
	}
	if input.SourceContext.Subject != "Pengembangan Sistem Reservasi" {
		t.Errorf("expected Subject 'Pengembangan Sistem Reservasi', got %q", input.SourceContext.Subject)
	}
	if len(input.BusinessGoals) != 2 {
		t.Errorf("expected 2 business goals, got %d", len(input.BusinessGoals))
	}
	if len(input.Actors) != 3 {
		t.Errorf("expected 3 actors, got %d", len(input.Actors))
	}
	if len(input.Channels) != 3 {
		t.Errorf("expected 3 channels, got %d", len(input.Channels))
	}

	marshaled, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	var roundTripInput BlueprintInput
	if err := json.Unmarshal(marshaled, &roundTripInput); err != nil {
		t.Fatalf("failed to unmarshal marshaled input: %v", err)
	}

	if !reflect.DeepEqual(input, roundTripInput) {
		t.Errorf("round trip input does not match original input")
	}
}

func TestBlueprintConstantsAndStatus(t *testing.T) {
	// Section Status constants
	if StatusPending != "pending" {
		t.Errorf("expected StatusPending 'pending', got %q", StatusPending)
	}
	if StatusGenerating != "generating" {
		t.Errorf("expected StatusGenerating 'generating', got %q", StatusGenerating)
	}
	if StatusCompleted != "completed" {
		t.Errorf("expected StatusCompleted 'completed', got %q", StatusCompleted)
	}
	if StatusFailed != "failed" {
		t.Errorf("expected StatusFailed 'failed', got %q", StatusFailed)
	}
	if StatusManualEdit != "manual_edit" {
		t.Errorf("expected StatusManualEdit 'manual_edit', got %q", StatusManualEdit)
	}
	if StatusValidated != "validated" {
		t.Errorf("expected StatusValidated 'validated', got %q", StatusValidated)
	}

	// Publication and Completeness Statuses
	if PublicationStatusDraft != "draft" {
		t.Errorf("expected PublicationStatusDraft 'draft', got %q", PublicationStatusDraft)
	}
	if PublicationStatusPublished != "published" {
		t.Errorf("expected PublicationStatusPublished 'published', got %q", PublicationStatusPublished)
	}
	if PublicationStatusSuperseded != "superseded" {
		t.Errorf("expected PublicationStatusSuperseded 'superseded', got %q", PublicationStatusSuperseded)
	}
	if CompletenessStatusPartial != "partial" {
		t.Errorf("expected CompletenessStatusPartial 'partial', got %q", CompletenessStatusPartial)
	}
	if CompletenessStatusComplete != "complete" {
		t.Errorf("expected CompletenessStatusComplete 'complete', got %q", CompletenessStatusComplete)
	}

	// Blueprint Stage constants
	if StageContextAnalysis != "context_analysis" {
		t.Errorf("expected StageContextAnalysis 'context_analysis', got %q", StageContextAnalysis)
	}
	if StageJourney != "journey" {
		t.Errorf("expected StageJourney 'journey', got %q", StageJourney)
	}
	if StageArchitecture != "architecture" {
		t.Errorf("expected StageArchitecture 'architecture', got %q", StageArchitecture)
	}
	if StageStateMachine != "state_machine" {
		t.Errorf("expected StageStateMachine 'state_machine', got %q", StageStateMachine)
	}
	if StageWorkflow != "workflow" {
		t.Errorf("expected StageWorkflow 'workflow', got %q", StageWorkflow)
	}
	if StageWBS != "wbs" {
		t.Errorf("expected StageWBS 'wbs', got %q", StageWBS)
	}
	if StageCrossValidation != "cross_validation" {
		t.Errorf("expected StageCrossValidation 'cross_validation', got %q", StageCrossValidation)
	}
}

func TestValidationIssueAndReportTypes(t *testing.T) {
	issue := ValidationIssue{
		Code:        "ERR_DUPLICATE_ID",
		Severity:    "BLOCKING_ERROR",
		Section:     "architecture",
		Field:       "/architecture/nodes/1/id",
		Path:        "/architecture/nodes/1/id",
		TargetID:    "node-payment",
		Message:     "Duplicate ID detected: node-payment",
		Blocking:    true,
		Remediation: "Ensure all node IDs are unique.",
	}

	issueData, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("failed to marshal ValidationIssue: %v", err)
	}

	var roundTripIssue ValidationIssue
	if err := json.Unmarshal(issueData, &roundTripIssue); err != nil {
		t.Fatalf("failed to unmarshal ValidationIssue: %v", err)
	}
	if roundTripIssue.Code != "ERR_DUPLICATE_ID" || !roundTripIssue.Blocking {
		t.Errorf("issue unmarshal mismatch: %+v", roundTripIssue)
	}

	stageResult := BlueprintStageResult{
		Stage:      StageJourney,
		Status:     StatusCompleted,
		StartedAt:  time.Now(),
		DurationMS: 150,
		RetryCount: 0,
		Issues:     []ValidationIssue{issue},
	}

	report := BlueprintBatchReport{
		SchemaVersion: "1.0",
		Fixture:       "test-fixture.json",
		Provider:      "mock",
		Model:         "deterministic-rnd-v1",
		PromptVersion: "blueprint-prompt-v1",
		Status:        "success",
		Stages: []BlueprintStageResult{
			stageResult,
		},
		Summary: BlueprintReportSummary{
			TotalStages:     7,
			CompletedStages: 7,
			BlockingErrors:  0,
			WarningsCount:   1,
			Passed:          true,
		},
	}

	reportData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal BlueprintBatchReport: %v", err)
	}

	var roundTripReport BlueprintBatchReport
	if err := json.Unmarshal(reportData, &roundTripReport); err != nil {
		t.Fatalf("failed to unmarshal BlueprintBatchReport: %v", err)
	}

	if roundTripReport.Status != "success" || len(roundTripReport.Stages) != 1 {
		t.Errorf("report unmarshal mismatch: %+v", roundTripReport)
	}
}
