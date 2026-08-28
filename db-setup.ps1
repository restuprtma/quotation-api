# quotation-api/db-setup.ps1
Write-Host "Running migrations..."
$dbUrl = "postgresql://postgres:postgres@localhost:5432/tuai?sslmode=disable"
migrate -path internal/database/migrations/core -database "$dbUrl&x-migrations-table=schema_migrations_core" up
migrate -path internal/database/migrations/quotation -database "$dbUrl&x-migrations-table=schema_migrations_quotation" up
migrate -path internal/database/migrations/contract -database "$dbUrl&x-migrations-table=schema_migrations_contract" up

Write-Host "Running core seeders..."
$seeders = Get-ChildItem -Path internal\database\seeders\core -Filter *.sql | Sort-Object Name
$env:PGPASSWORD="postgres"
foreach ($seeder in $seeders) {
    Write-Host "Running seeder: $($seeder.Name)"
    psql -v ON_ERROR_STOP=1 -h localhost -p 5432 -U postgres -d tuai -f $seeder.FullName
}
Write-Host "✅ Database setup completed for Windows!"
