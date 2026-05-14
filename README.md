# ffscan_GO
ffscan re-written in GO

## Rebuild platform specific binaries (Optional, works well even without this)

## Windows
set GOOS=windows
set GOARCH=amd64
go build -o releases/ffscan-windows-amd64.exe .

## macOS
set GOOS=darwin
set GOARCH=amd64
go build -o releases/ffscan-darwin-amd64 .

## Linux
set GOOS=linux
set GOARCH=amd64
go build -o releases/ffscan-linux-amd64 .
