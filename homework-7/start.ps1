$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go не найден. Установите Go 1.22 или новее и откройте PowerShell заново.'
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker не найден. Установите Docker Desktop, запустите его и повторите команду.'
}
docker info --format '{{.ServerVersion}}'
if ($LASTEXITCODE -ne 0) { throw 'Docker недоступен. Запустите Docker Desktop и дождитесь готовности.' }

docker compose up -d --wait
if ($LASTEXITCODE -ne 0) { throw 'Не удалось запустить PostgreSQL и Redis. Проверьте доступность портов 5432 и 6379.' }

if (-not $env:DATABASE_URL) { $env:DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable' }
if (-not $env:REDIS_ADDR) { $env:REDIS_ADDR = 'localhost:6379' }
if (-not $env:REDIS_DB) { $env:REDIS_DB = '0' }

$goose = Get-Command goose -ErrorAction SilentlyContinue
if ($goose) {
    $goosePath = $goose.Source
} else {
    $goBin = go env GOBIN
    if (-not $goBin) {
        $goPath = (go env GOPATH).Split([System.IO.Path]::PathSeparator)[0]
        $goBin = Join-Path $goPath 'bin'
    }
    $goosePath = Join-Path $goBin 'goose.exe'
    if (-not (Test-Path $goosePath)) {
        go install -tags 'no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.22.1
        if ($LASTEXITCODE -ne 0) { throw 'Не удалось установить Goose.' }
    }
}
& $goosePath -dir .\ledger\migrations postgres $env:DATABASE_URL up
if ($LASTEXITCODE -ne 0) { throw 'Миграции завершились с ошибкой.' }

Set-Location (Join-Path $PSScriptRoot 'gateway')
go run .
if ($LASTEXITCODE -ne 0) { throw 'Gateway завершился с ошибкой.' }
