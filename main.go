package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WhatsAppApi struct {
	Client     *whatsmeow.Client
	QR         string
	Connected  bool
	Number     string
	WebhookURL string
	StartTime  time.Time
	mu         sync.RWMutex
}

var waAPI = &WhatsAppApi{}

func main() {
	dbLog := waLog.Stdout("Database", "ERROR", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:sessions.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", true)
	waAPI.Client = whatsmeow.NewClient(deviceStore, clientLog)
	waAPI.Client.AddEventHandler(eventHandler)
	waAPI.WebhookURL = os.Getenv("WEBHOOK_URL")
	waAPI.StartTime = time.Now()

	// Initial connect if session exists
	if waAPI.Client.Store.ID != nil {
		err := waAPI.Client.Connect()
		if err != nil {
			log.Printf("Failed to connect: %v", err)
		}
	} else {
		go startLogin()
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		BodyLimit:             50 * 1024 * 1024, // 50MB limit
	})
	app.Use(cors.New())

	// 1. Main UI Dashboard
	app.Get("/", func(c *fiber.Ctx) error {
		waAPI.mu.RLock()
		defer waAPI.mu.RUnlock()
		c.Set("Content-Type", "text/html; charset=utf-8")

		if waAPI.Connected {
			return c.SendString(`
				<div style="text-align:center; font-family:sans-serif; margin-top:50px;">
					<h1 style="color:#2577D3;">✅ Go Api is Connected</h1>
					<p>Linked Number: <strong>` + waAPI.Number + `</strong></p>
					<p>Pure Go API is ready.</p>
				</div>
			`)
		} else if waAPI.QR != "" {
			return c.SendString(`
				<div style="text-align:center; font-family:sans-serif; margin-top:50px;">
					<h1>📱 Scan WhatsApp QR (Go)</h1>
					<img src="/qr" style="border:10px solid #f0f0f0; border-radius:10px; padding:10px;" />
					<p>Refresh page if QR expires.</p>
					<script>setTimeout(() => location.reload(), 30000);</script>
				</div>
			`)
		}
		return c.SendString(`
			<div style="text-align:center; font-family:sans-serif; margin-top:50px;">
				<h1>🔄 Initializing...</h1>
				<p>Please wait while the waAPI prepares the QR code.</p>
				<script>setTimeout(() => location.reload(), 2000);</script>
			</div>
		`)
	})

	// 2. Direct QR PNG
	app.Get("/qr", func(c *fiber.Ctx) error {
		waAPI.mu.RLock()
		code := waAPI.QR
		waAPI.mu.RUnlock()
		if code == "" {
			return c.Status(404).SendString("QR not available")
		}
		png, _ := qrcode.Encode(code, qrcode.Medium, 256)
		c.Set("Content-Type", "image/png")
		return c.Send(png)
	})

	// 3. Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		waAPI.mu.RLock()
		defer waAPI.mu.RUnlock()
		return c.JSON(fiber.Map{
			"status":       "ok",
			"connected":    waAPI.Connected,
			"waAPI_number": waAPI.Number,
			"runtime":      "golang",
		})
	})

	api := app.Group("/api")

	// 4. API QR Base64
	api.Get("/qr", func(c *fiber.Ctx) error {
		waAPI.mu.RLock()
		code := waAPI.QR
		waAPI.mu.RUnlock()
		if code != "" {
			png, _ := qrcode.Encode(code, qrcode.Medium, 256)
			base64Img := base64.StdEncoding.EncodeToString(png)
			return c.JSON(fiber.Map{
				"status": "scanning",
				"qr":     "data:image/png;base64," + base64Img,
			})
		}
		return c.JSON(fiber.Map{"status": "connected", "waAPI_number": waAPI.Number})
	})

	// 5. API Start
	api.Post("/start", func(c *fiber.Ctx) error {
		go startLogin()
		return c.JSON(fiber.Map{"success": true, "message": "API starting..."})
	})

	// 6. Send Text Message
	api.Post("/send-message", func(c *fiber.Ctx) error {
		var req struct {
			Number  string `json:"number"`
			Message string `json:"message"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		jid := parseJID(req.Number)

		// Auto-Typing
		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		time.Sleep(1 * time.Second) // Natural delay

		msg := &waE2E.Message{Conversation: proto.String(req.Message)}
		_, err := waAPI.Client.SendMessage(context.Background(), jid, msg)

		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresencePaused, types.ChatPresenceMediaText)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// 7. Send Image
	api.Post("/send-image", func(c *fiber.Ctx) error {
		var req struct {
			Number  string `json:"number"`
			URL     string `json:"url"`
			Caption string `json:"caption"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		jid := parseJID(req.Number)
		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)

		data, err := downloadFile(req.URL)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to download image: " + err.Error()})
		}

		resp, err := waAPI.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to upload image: " + err.Error()})
		}

		msg := &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:       proto.String(req.Caption),
				Mimetype:      proto.String(http.DetectContentType(data)),
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				FileLength:    proto.Uint64(uint64(len(data))),
				FileSHA256:    resp.FileSHA256,
				FileEncSHA256: resp.FileEncSHA256,
			},
		}

		_, err = waAPI.Client.SendMessage(context.Background(), jid, msg)
		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresencePaused, types.ChatPresenceMediaText)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// 8. Send Video
	api.Post("/send-video", func(c *fiber.Ctx) error {
		var req struct {
			Number  string `json:"number"`
			URL     string `json:"url"`
			Caption string `json:"caption"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		jid := parseJID(req.Number)
		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaAudio) // Video takes time

		data, err := downloadFile(req.URL)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to download video: " + err.Error()})
		}

		resp, err := waAPI.Client.Upload(context.Background(), data, whatsmeow.MediaVideo)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to upload video: " + err.Error()})
		}

		msg := &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				Caption:       proto.String(req.Caption),
				Mimetype:      proto.String(http.DetectContentType(data)),
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				FileLength:    proto.Uint64(uint64(len(data))),
				FileSHA256:    resp.FileSHA256,
				FileEncSHA256: resp.FileEncSHA256,
			},
		}

		_, err = waAPI.Client.SendMessage(context.Background(), jid, msg)
		waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresencePaused, types.ChatPresenceMediaAudio)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// 9. Send Audio
	api.Post("/send-audio", func(c *fiber.Ctx) error {
		var req struct {
			Number string `json:"number"`
			URL    string `json:"url"`
			PTT    bool   `json:"ptt"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		jid := parseJID(req.Number)
		if req.PTT {
			waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
			time.Sleep(1500 * time.Millisecond) // Recording feel
		} else {
			waAPI.Client.SendChatPresence(context.Background(), jid, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
		}

		data, err := downloadFile(req.URL)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to download audio: " + err.Error()})
		}

		resp, err := waAPI.Client.Upload(context.Background(), data, whatsmeow.MediaAudio)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to upload audio: " + err.Error()})
		}

		msg := &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String("audio/mpeg"),
				FileLength:    proto.Uint64(uint64(len(data))),
				FileSHA256:    resp.FileSHA256,
				FileEncSHA256: resp.FileEncSHA256,
				PTT:           proto.Bool(req.PTT),
			},
		}

		_, err = waAPI.Client.SendMessage(context.Background(), jid, msg)
		return c.JSON(fiber.Map{"success": true})
	})

	// 10. Presence (Typing Status)
	api.Post("/presence", func(c *fiber.Ctx) error {
		var req struct {
			Number string `json:"number"`
			Status string `json:"status"` // "composing" or "paused"
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		jid := parseJID(req.Number)
		presence := types.ChatPresenceComposing
		if req.Status == "paused" {
			presence = types.ChatPresencePaused
		}
		waAPI.Client.SendChatPresence(context.Background(), jid, presence, types.ChatPresenceMediaText)
		return c.JSON(fiber.Map{"success": true})
	})

	// Keep-Alive Loop
	go func() {
		for {
			time.Sleep(30 * time.Second)
			if waAPI.Client != nil && waAPI.Client.IsConnected() {
				waAPI.Client.SendPresence(context.Background(), types.PresenceAvailable)
			}
		}
	}()

	// OS Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down Go Api...")
		waAPI.Client.Disconnect()
		os.Exit(0)
	}()

	fmt.Println("🚀 Go WhatsApp API running on port 3000")
	log.Fatal(app.Listen(":3000"))
}

func startLogin() {
	if waAPI.Client.IsConnected() {
		return
	}
	qrChan, _ := waAPI.Client.GetQRChannel(context.Background())
	err := waAPI.Client.Connect()
	if err != nil {
		return
	}
	for evt := range qrChan {
		if evt.Event == "code" {
			waAPI.mu.Lock()
			waAPI.QR = evt.Code
			waAPI.mu.Unlock()
			qr, _ := qrcode.New(evt.Code, qrcode.Medium)
			fmt.Println("\n" + qr.ToSmallString(false))
			fmt.Println("New QR available in browser or /qr")
		}
	}
}

func eventHandler(evt interface{}) {
	switch evt := evt.(type) {
	case *events.Connected:
		waAPI.mu.Lock()
		waAPI.Connected = true
		waAPI.QR = ""
		waAPI.Number = waAPI.Client.Store.ID.User
		waAPI.mu.Unlock()
		fmt.Printf("✅ Go Api Connected: %s\n", waAPI.Number)
	case *events.LoggedOut:
		waAPI.mu.Lock()
		waAPI.Connected = false
		waAPI.mu.Unlock()
	case *events.Message:
		// Ignore status broadcasts and unwanted messages
		if evt.Info.IsFromMe || evt.Info.IsGroup || evt.Info.Category == "peer_broadcast_notification" {
			return
		}

		// Resolve Contact (Handle LID -> Phone Number)
		sender := evt.Info.Sender
		realJID := sender
		pushName := evt.Info.PushName

		// If it's an LID (Linked Device ID), try to find the real phone number
		if sender.Server == "lid" {
			if waAPI.Client.Store.LIDs != nil {
				if pn, err := waAPI.Client.Store.LIDs.GetPNForLID(context.Background(), sender); err == nil && !pn.IsEmpty() {
					realJID = pn
				}
			}
			if waAPI.Client.Store.Contacts != nil {
				if contact, err := waAPI.Client.Store.Contacts.GetContact(context.Background(), sender); err == nil && contact.Found {
					if contact.PushName != "" {
						pushName = contact.PushName
					}
				} else if contact, err := waAPI.Client.Store.Contacts.GetContact(context.Background(), realJID); err == nil && contact.Found {
					if contact.PushName != "" {
						pushName = contact.PushName
					}
				}
			}
		}

		// Start Typing Indicator immediately
		waAPI.Client.SendChatPresence(context.Background(), sender, types.ChatPresenceComposing, types.ChatPresenceMediaText)

		// Extract pure phone number
		phone := strings.Split(realJID.User, ":")[0]

		// Prepare payload
		payload := map[string]interface{}{
			"event":     "message",
			"id":        evt.Info.ID,
			"push_name": pushName,
			"number":    phone,
			"jid":       realJID.String(),
			"message":   evt.Message.GetConversation(),
			"timestamp": evt.Info.Timestamp.Unix(),
		}

		// Extended Text Message support
		if payload["message"] == "" {
			payload["message"] = evt.Message.GetExtendedTextMessage().GetText()
		}

		fmt.Printf("📩 [%s] %s: %s\n", payload["push_name"], payload["number"], payload["message"])

		if waAPI.WebhookURL != "" {
			go sendWebhook(payload)
		} else {
			fmt.Println(" (No Webhook Configured)")
		}
	}
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 20,
	},
}

func sendWebhook(payload map[string]interface{}) {
	jsonBody, _ := json.Marshal(payload)

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("POST", waAPI.WebhookURL, bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Printf("✅ Webhook Sent: %s -> %s\n", payload["number"], waAPI.WebhookURL)
				return
			}
			fmt.Printf("⚠️ Webhook Failed (Status %d): Retrying...\n", resp.StatusCode)
		} else {
			fmt.Printf("❌ Webhook Error: %v. Retrying...\n", err)
		}
		time.Sleep(500 * time.Millisecond) // Faster retry
	}
	fmt.Println("❌ Webhook Failed after 3 attempts.")
}

func parseJID(arg string) types.JID {
	if arg == "" {
		return types.EmptyJID
	}
	if !strings.Contains(arg, "@") {
		return types.NewJID(arg, types.DefaultUserServer)
	}
	recipient, _ := types.ParseJID(arg)
	return recipient
}

func downloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
