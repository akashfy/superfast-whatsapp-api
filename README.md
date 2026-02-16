# ⚡️ SuperFast WhatsApp Api

> **The World's Fastest WhatsApp API.** Built with **Golang 1.26** & **Fiber**. 🚀

![Go](https://img.shields.io/badge/Language-Go_1.26-00ADD8?style=for-the-badge&logo=go)
![Docker](https://img.shields.io/badge/Container-Alpine_Linux-0db7ed?style=for-the-badge&logo=docker)
![Status](https://img.shields.io/badge/Status-Production_Ready-success?style=for-the-badge)

A **blazingly fast**, **low-latency**, and **lightweight** WhatsApp API. Designed for high-throughput messaging with **zero** external runtime dependencies.

---

### 🔥 Why This Api?

| Feature | Description |
| :--- | :--- |
| **🚀 Instant Boot** | Starts in **<100ms**. Ready to send messages immediately. |
| **💾 Low Memory** | Uses **<15MB RAM** idle. Extremely efficient compared to Node.js/Python. |
| **🐳 Tiny Docker Image** | **<25MB** total compressed size. Deploys anywhere in seconds. |
| **🛡️ Type-Safe** | Built with **Go** for maximum stability and concurrency. |

---

### 🐳 Docker Deployment (Instant)

Pull the pre-built image directly from Docker Hub:
```bash
docker pull akashyadav758/superfast-whatsapp-api:latest
```

Or use **Docker Compose**:
```bash
# 1. Download docker-compose.yml
# 2. Run:
docker-compose up -d
```

### 🚀 Manual Quick Start
```bash
# Clone the repository
git clone https://github.com/akashfy/superfast-whatsapp-api.git
cd superfast-whatsapp-api
```

The API listens on **`:3000`**.

---

### 📡 SuperFast API Endpoints

#### 1. 🟢 System Health
`GET /health`
> **Response:** `{"status":"ok", "latency":"2ms"}`
> **Use:** Real-time uptime monitoring.

#### 2. 📨 Send Text (Instant)
`POST /api/send-message`
```json
{
  "number": "919876543210",
  "message": "⚡️ This message was sent via Go!"
}
```

#### 3. 📸 Send Media (Image/Video)
`POST /api/send-image`
```json
{
  "number": "919876543210",
  "url": "https://example.com/image.jpg",
  "caption": "Look how fast this arrived! 🚀"
}
```

#### 4. 🎙️ Send Voice Note
`POST /api/send-audio`
```json
{
  "number": "919876543210",
  "url": "https://example.com/audio.mp3",
  "ptt": true
}
```

---

### 🔌 Incoming Webhooks (2-Way Communication)
Connect your AI bots or backend services using a robust, real-time webhook system.

#### Configuration
Set the `WEBHOOK_URL` environment variable to start receiving messages:
- **Docker Compose:** Add `- WEBHOOK_URL=http://your-server:5000/webhook` in environment section.
- **Manual:** `export WEBHOOK_URL=http://your-server:5000/webhook`

#### Features
- **Smart Resolution:** Automatically maps WhatsApp LIDs to real phone numbers.
- **Push Name Support:** Receives the sender's public WhatsApp profile name.
- **Reliable Delivery:** Automatic **3x retry** logic if your webhook server is temporarily down.

#### 📩 Webhook Payload Structure
```json
{
  "event": "message",
  "id": "3EB0...",
  "number": "916262255556",
  "push_name": "Akash Yadav",
  "message": "Hi, how are you?",
  "timestamp": 1708105000,
  "jid": "916262255556@s.whatsapp.net"
}
```

---

### 🛠️ Tech Stack
- **Language:** Golang (1.26)
- **Web Framework:** Fiber (v2) - *Fastest Go Web Framework*
- **WhatsApp Engine:** WhatsMeow - *Reverse Engineering Masterpiece*
- **Database:** SQLite (Embedded, Zero-Config)

---

### 🤝 Contributing
Feel free to open an issue or submit a PR. Let's make it even faster!

License: MIT
