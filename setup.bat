@echo off
REM Dock Otter Setup Script for Windows

echo 🦦 Dock Otter Setup Script
echo ==========================

REM Check if Docker is running
docker info >nul 2>&1
if errorlevel 1 (
    echo ❌ Docker is not running. Please start Docker Desktop first.
    pause
    exit /b 1
)

REM Check Docker Compose availability
docker compose version >nul 2>&1
if not errorlevel 1 (
    set COMPOSE_CMD=docker compose
    goto :compose_found
)

docker-compose --version >nul 2>&1
if not errorlevel 1 (
    set COMPOSE_CMD=docker-compose
    goto :compose_found
)

echo ❌ Docker Compose is not available. Please install Docker Compose.
pause
exit /b 1

:compose_found
echo ✅ Using: %COMPOSE_CMD%

REM Create .env file if it doesn't exist
if not exist .env (
    echo 📝 Creating .env file from template...
    copy .env.example .env
    echo ⚠️  Please edit .env file with your API keys before continuing!
    echo    Required: DOKPLOY_API_KEY and PANGOLIN_TOKEN
    pause
    exit /b 0
)

REM Create logs directory
if not exist logs mkdir logs

REM Build and start
echo 🚀 Building and starting Dock Otter...
%COMPOSE_CMD% up -d --build

echo.
echo ✅ Dock Otter is starting up!
echo.
echo 📊 Check status:
echo    %COMPOSE_CMD% logs -f dock-otter
echo.
echo 🏥 Health check:
echo    curl http://localhost:8080/health
echo.
echo 🛑 To stop:
echo    %COMPOSE_CMD% down
echo.
pause