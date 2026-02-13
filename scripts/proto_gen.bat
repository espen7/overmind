@echo off
setlocal

:: Ensure output directories exist
if not exist "pkg\pb\kit" mkdir "pkg\pb\kit"
if not exist "pkg\pb\gateway" mkdir "pkg\pb\gateway"

echo Generating Protobuf files...

:: 1. Check project-local protoc (standard structure: tools/protoc/bin/protoc.exe)
if exist "tools\protoc\bin\protoc.exe" (
    set "PROTOC_CMD=tools\protoc\bin\protoc.exe"
    echo Using local protoc: %PROTOC_CMD%
) else if exist "tools\protoc\protoc.exe" (
    :: 1b. Check flat structure (tools/protoc/protoc.exe)
    set "PROTOC_CMD=tools\protoc\protoc.exe"
    echo Using local protoc: %PROTOC_CMD%
) else (
    :: 2. Fallback to PATH
    where protoc >nul 2>nul
    if %ERRORLEVEL% EQU 0 (
        set "PROTOC_CMD=protoc"
        echo Using global protoc from PATH
    ) else (
        echo Error: protoc not found in 'tools\protoc\bin\' OR in PATH.
        echo Please download protoc-win64.zip and extract to 'tools\protoc\', or install globally.
        exit /b 1
    )
)

:: Check if protoc-gen-go is in PATH (required for --go_out)
where protoc-gen-go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo Error: protoc-gen-go not found. Please run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    exit /b 1
)

:: Collect all proto files (relative paths)
set "PROTO_FILES="

cd api\proto\kit
for %%f in (*.proto) do call set "PROTO_FILES=%%PROTO_FILES%% api/proto/kit/%%f"
cd ..\..\..

cd api\proto\gateway
for %%f in (*.proto) do call set "PROTO_FILES=%%PROTO_FILES%% api/proto/gateway/%%f"
cd ..\..\..

if "%PROTO_FILES%"=="" (
    echo Error: No .proto files found in api\proto\kit or api\proto\gateway.
    exit /b 1
)

:: Run protoc
echo Running: "%PROTOC_CMD%" -I=. --go_out=. --go_opt=paths=source_relative %PROTO_FILES%
"%PROTOC_CMD%" -I=. --go_out=. --go_opt=paths=source_relative %PROTO_FILES%

if %ERRORLEVEL% NEQ 0 (
    echo Protoc generation failed.
    exit /b 1
)

echo Moving generated files...
if exist "api\proto\kit\*.pb.go" (
    move /Y "api\proto\kit\*.pb.go" "pkg\pb\kit\" >nul
)
if exist "api\proto\gateway\*.pb.go" (
    move /Y "api\proto\gateway\*.pb.go" "pkg\pb\gateway\" >nul
)

echo Protobuf generation complete.
exit /b 0
