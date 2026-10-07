# Tuai Backend

Backend API untuk Tuai Quotation & Blueprint Generator menggunakan Golang + Gin + PostgreSQL.

## Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Redis (untuk rate limiting dan caching)
- golang-migrate CLI (opsional untuk migration manual)
- air CLI (untuk hot reload development)

## Environment Configuration

Salin `.env.example` ke `.env`:

```bash
cp .env.example .env
```

Pastikan variabel penting berikut dikonfigurasi:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=tuai

# AI Security & Encryption
# Kunci enkripsi AES-256 32-byte untuk API Key provider AI yang disimpan di database
AI_CREDENTIAL_ENCRYPTION_KEY=your-32-byte-secret-key-here-123456

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# App
PORT=8080
APP_ENV=development
```

## Installation & Setup

### 1. Install Dependencies

```bash
go mod download
```

### 2. Setup Database (Fresh Disposable DB Setup)

Menjalankan migrasi database (`core`, `quotation`, `contract`) dan mengisi seeder master rate cards:

```bash
make db-setup
```

Atau jalankan langkah-langkah secara terpisah:

```bash
# Jalankan migrasi seluruh modul
make migrate-up

# Seed master data (core & quotation rate cards)
make seed
```

### 3. Check Migration Version

```bash
make migrate-version
```

### 4. Creating New Migration

```bash
# Buat migration baru untuk modul tertentu (core, quotation, contract)
make migrate-create NAME=create_feature_table MODULE=quotation
```

## Running the Application

### Development Mode (with Hot Reload) 🔥

```bash
make dev
```

### Manual Run

```bash
go run cmd/api/main.go
```

### Build & Run Production

```bash
make build
./bin/tuai-api
```

### Testing & Verification

```bash
# Run unit & integration test matrix dengan race detector
go test -race ./internal/modules/quotation/...
go test ./internal/modules/contract/...

# Build verification
go build ./...
```

## Project Structure

```
├── cmd/
│   └── api/                   # Application entry point
├── internal/
│   ├── config/                # Environment configuration
│   ├── database/              # Database connection & SQL migrations (core, quotation, contract)
│   ├── middleware/            # Auth, CORS, tenant isolation middleware
│   ├── modules/
│   │   ├── core/              # Core identity (company, user, auth, role, client)
│   │   ├── quotation/         # AI generator, TipTap revisions, blueprints, sharing
│   │   └── contract/          # Legally-binding signed contracts
│   ├── router/                # Route registrations & audit tests
│   └── shared/                # Shared utilities & response helpers
└── pkg/                       # Reusable packages (crypto, validator, logger)
```

## Canonical API Architecture

### 1. Internal Protected Endpoints (Bearer JWT)

#### AI Configuration & Skills
- `GET /quotation/v1/ai/settings` - Konfigurasi active AI provider, model, dan key hint
- `PUT /quotation/v1/ai/settings` - Update model, provider, dan enkripsi API key
- `GET /quotation/v1/ai/skills` - Daftar skills aktif (`/quotation`, `/blueprint`, dll.)

#### Quotation Management
- `POST /quotation/v1/quotations/generate` - AI streaming/sync quotation generation (HTML TipTap)
- `PUT /quotation/v1/quotations/:id/draft` - Optimistic draft save (HTML content & legal override audit)
- `POST /quotation/v1/quotations/:id/publish` - Publish draft menjadi active published revision
- `GET /quotation/v1/quotations/:id/revisions` - Riwayat revisi quotation
- `POST /quotation/v1/quotations/:id/revisions` - Branching revisi baru dari quotation

#### Blueprint Management & Sharing
- `POST /quotation/v1/blueprints/generate` - Blueprint pipeline generator dari quotation source
- `PUT /quotation/v1/blueprints/:id/sections/:section/draft` - Draft save visual section
- `POST /quotation/v1/blueprints/:id/publish` - Publish blueprint revision (lengkap / parsial)
- `POST /quotation/v1/blueprints/:id/share-links` - Buat secure link publik untuk programmer
- `GET /quotation/v1/blueprints/:id/share-links` - Daftar link share aktif
- `DELETE /quotation/v1/blueprints/:id/share-links/:link_id` - Revoke link share

### 2. Public Endpoints (No Auth)

#### Client Quotation & Digital Signature
- `GET /p/q/:public_token` - Akses quotation published aktif (sanitized HTML, stable link UUID)
- `POST /p/q/:public_token/sign` - Bubuhkan tanda tangan digital (idempoten & race-safe)

#### Programmer Blueprint View
- `GET /p/blueprint/:token` - Akses blueprint teknis (terenkapsulasi, zero commercial/budget data)

## Security & Isolation Principles

1. **Multi-Tenant Isolation**: Seluruh kueri database internal dibatasi strictly oleh `company_id` dari JWT token.
2. **Secret Non-Leakage**: Kunci API AI disimpan terenkripsi AES-256; response hanya menampilkan hint 4 digit terakhir; log sistem secara ketat menyamarkan payload sensitif.
3. **Public Token Entropy**: Token publik `/p/q/` dan `/p/blueprint/` dibangkitkan via CSPRNG (minimal 32 byte entropy), disimpan dalam format hash SHA-256, tanpa plaintext di DB atau log.
4. **HTML Sanitization**: Konten HTML TipTap melalui sanitizer UGC ketat sebelum disimpan dan disajikan ke client publik.
