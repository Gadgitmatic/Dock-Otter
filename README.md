# 🦦 Dock Otter

**Single binary adapter** that connects Dokploy to Pangolin for seamless domain management on your VPS.

## 🎯 What it does

- 📡 Reads Dokploy apps + domains via REST API
- 🔄 Generates Pangolin Blueprints automatically  
- 🚀 Pushes them to Pangolin's API
- 📝 Logs everything (safe, read-only operations)
- 🌉 Bridges Docker networks seamlessly
- ⚡ Manual sync for existing apps

## 🚫 What it doesn't do

- ❌ Modify Dokploy configs
- ❌ Change DNS records  
- ❌ Impact existing Traefik setup

---

## 🚀 VPS Installation

### 📥 Step 1: Get the Code

```bash
# SSH into your VPS
ssh user@your-vps-ip

# Clone the repo
git clone <this-repo-url>
cd dock-otter
```

### 🔑 Step 2: Configure API Keys

```bash
# Copy config template
cp .env.example .env

# Edit with your API keys
nano .env
```

Add your API credentials:
```bash
# Dokploy API (get from Dokploy admin panel)
DOKPLOY_API_KEY=your-dokploy-api-key

# Pangolin API (get from Pangolin dashboard)  
PANGOLIN_TOKEN=your-pangolin-bearer-token

# URLs (should work as-is for standard setups)
DOKPLOY_URL=http://dokploy:3000
PANGOLIN_URL=http://pangolin:3001
```

### 🐳 Step 3: Deploy

```bash
# Build and start
docker-compose up -d --build

# Check logs
docker-compose logs -f dock-otter
```

### ✅ Step 4: Verify

Look for these success messages:
```
🦦 Dock Otter starting up...
🔐 Dokploy auth: API key, Pangolin auth: Bearer token
🔍 Testing API connectivity...
✅ Dokploy connected - found 2 projects, 3 apps, 5 domains
✅ Pangolin API accessible
🔄 Syncing apps from Dokploy...
🔧 Creating Pangolin resource for example.com -> myapp:3000 (https)
✅ Pangolin resource created: myapp-example-com
✅ Sync completed - processed 2, skipped 1, errors 0
```

---

## ⚙️ Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DOKPLOY_URL` | `http://dokploy:3000` | 🐳 Dokploy API endpoint |
| `DOKPLOY_API_KEY` | - | 🔑 Dokploy API key (recommended) |
| `DOKPLOY_TOKEN` | - | 🎫 Or Bearer token |
| `DOKPLOY_SESSION` | - | 🍪 Or session cookie |
| `PANGOLIN_URL` | `http://pangolin:3001` | 🦎 Pangolin API endpoint |
| `PANGOLIN_TOKEN` | - | 🎫 Pangolin Bearer token (preferred) |
| `PANGOLIN_API_KEY` | - | 🔑 Or API key |
| `POLL_INTERVAL` | `30s` | ⏰ How often to sync |
| `RUN_ONCE` | `false` | ⚡ Manual execution mode |
| `FORCE_SYNC` | `false` | 🔄 Force re-sync existing |
| `RETRY_ATTEMPTS` | `3` | 🔁 API retry count |
| `RETRY_DELAY` | `5s` | ⏳ Retry delay |

---

## 🏗️ How It Works

```
📦 Dokploy Apps ──→ 🦦 Dock Otter ──→ 📋 Pangolin Blueprints ──→ 🦎 Gerbil ──→ 🌐 Internet
      (3000)              (bridge)              (3001)              (80/443)
```

1. 📡 **Dock Otter polls Dokploy** every 30s for running apps with domains
2. 🔧 **Generates Pangolin Blueprints** for each app/domain pair
3. 🚀 **Pushes to Pangolin API** which configures Gerbil automatically
4. 🔒 **Gerbil exposes domains** externally with TLS certificates
5. 🌊 **Traffic flows**: Internet → Gerbil → Dokploy apps (internal ports)

### 📋 Example Blueprint

For Dokploy app "myapp" with domain "example.com":

```yaml
proxy-resources:
  - name: myapp-example-com
    protocol: http
    full-domain: example.com
    ssl: true
    enabled: true
    targets:
      - hostname: myapp-container
        port: 3000
        method: https
        enabled: true
        path: /
```

---

## 🛠️ Management

```bash
# 📋 View logs
docker-compose logs -f dock-otter

# 🏥 Check version and health
docker exec dock-otter /dock-otter --version
curl http://localhost:8080/health

# ⚡ Manual sync (existing apps)
docker exec dock-otter sh -c "RUN_ONCE=true /dock-otter"

# 🔄 Force re-sync all resources
docker exec dock-otter sh -c "RUN_ONCE=true FORCE_SYNC=true /dock-otter"

# 🔄 Restart
docker-compose restart dock-otter

# 🛑 Stop
docker-compose down

# 🔄 Update
git pull && docker-compose up -d --build
```

---

## 🔍 Troubleshooting

### 🌐 Connection Issues

```bash
# Check if services are running
docker ps | grep -E "(dokploy|pangolin)"

# Test network connectivity
docker exec dock-otter wget -qO- http://dokploy:3000 --timeout=5
docker exec dock-otter wget -qO- http://pangolin:3001 --timeout=5

# Check networks exist
docker network ls | grep -E "(shared-proxy|dokploy|pangolin)"
```

### 🔐 Authentication Issues

```bash
# Verify API keys in logs (without exposing secrets)
docker-compose logs dock-otter | grep "🔐"

# Test Dokploy API manually
curl -H "X-API-Key: YOUR_KEY" http://dokploy:3000/api/project/all

# Test Pangolin API manually  
curl -H "Authorization: Bearer YOUR_TOKEN" http://pangolin:3001/v1/docs
```

### 📦 No Apps Found

- Ensure apps are **running** in Dokploy (status: "done")
- Verify apps have **domains configured** in Dokploy
- Check Dokploy API returns data: `curl http://dokploy:3000/api/project/all`

---

## �️ SafetyF Features

- 👀 **Read-only** access to Dokploy
- 🔄 **Graceful retries** with exponential backoff
- 🏥 **Health monitoring** on port 8080
- 📝 **Comprehensive logging** with emoji indicators
- 🚫 **No destructive operations** - only creates Pangolin resources
- 🔒 **Secure container** - distroless, non-root user

---

## 📁 Project Structure

```
dock-otter/
├── 🐹 main.go              # Single binary (all code)
├── 📦 go.mod               # Dependencies  
├── ⚙️ .env.example         # Config template
├── 🐳 docker-compose.yml   # VPS deployment
├── 🏗️ Dockerfile           # Container build
├── 🚫 .dockerignore        # Build optimization
└── 📖 README.md            # This file
```

**That's it!** Clean, minimal structure optimized for Docker deployment. 🎯