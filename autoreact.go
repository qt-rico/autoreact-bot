package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
)

// ─── Colors ──────────────────────────────
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[1;31m"
	ColorGreen  = "\033[1;32m"
	ColorYellow = "\033[1;33m"
	ColorBlue   = "\033[1;34m"
	ColorCyan   = "\033[0;36m"
	ColorFatal  = "\033[1;41m"
)

// ─── Globals ─────────────────────────────
var (
	channelURL   = getEnv("CHANNEL_URL", "https://t.me/example")
	groupURL     = getEnv("GROUP_URL", "https://t.me/example_group")
	db           *sql.DB
	failCount    = make(map[string]int)
	mutex        sync.Mutex
	subscribers  = make(map[int64]struct{})   // chats to broadcast to
	broadcastMap = make(map[int64]bool)       // ownerID -> awaiting next msg
	ownerID      = int64(5290407067)          // only this ID can broadcast

	botInstances []*tgbotapi.BotAPI           // all bot instances
	botMutex     sync.RWMutex                 // protects botInstances

	emojis = []string{
		"❤️", "👍", "🔥", "🥰", "👏", "😁", "🤔", "🤯", "😱", "🤬", "😢", "🎉",
		"🤩", "🤮", "💩", "🙏", "👌", "🕊️", "🤡", "🥱", "🥴", "😍", "🐳", "❤️‍🔥",
		"🌚", "🌭", "💯", "🤣", "⚡", "🍌", "🏆", "💔", "🤨", "😐", "🍓", "🍾",
		"💋", "🖕", "😈", "😴", "😭", "🤓", "👻", "👨‍💻", "👀", "🎃", "🙈", "😇",
		"😨", "🤝", "✍️", "🤗", "🫡", "🎅", "🎄", "☃️", "💅", "🤪", "🗿", "🆒",
		"💘", "🙉", "🦄", "😘", "💊", "🙊", "😎", "👾", "🤷‍♂️", "🤷", "🤷‍♀️", "😡",
	}
)

// ─── Main ───────────────────────────────
func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println(ColorCyan + "🔁 Starting bot system..." + ColorReset)

	if err := initDB(); err != nil {
		log.Fatalf(ColorFatal+"💥 DB init failed: %v"+ColorReset, err)
	}

	tokens := strings.Split(os.Getenv("BOT_TOKENS"), ",")
	if len(tokens) == 0 || tokens[0] == "" {
		log.Fatal(ColorRed + "❌ BOT_TOKENS env var is required!" + ColorReset)
	}

	go startDummyServer()

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		wg.Add(1)
		go func(tok string) {
			defer wg.Done()
			if err := runBot(ctx, tok); err != nil {
				logError("runBot", tok, err)
			}
		}(token)
	}

	wg.Wait()
}

// ─── Utility ─────────────────────────────
func getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Printf(ColorYellow+"⚠️  %s not set, using default: %s"+ColorReset, key, fallback)
		return fallback
	}
	log.Printf(ColorGreen+"🔧 Loaded env: %s=%s"+ColorReset, key, val)
	return val
}

// ─── Dummy Server ────────────────────────
func startDummyServer() {
	port := getEnv("PORT", "10000")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println(ColorGreen + "✅ Health check received" + ColorReset)
		if _, err := w.Write([]byte("AFK bot is alive!")); err != nil {
			logError("write response", "http", err)
		}
	})
	log.Println(ColorBlue + "🌐 Dummy server on port " + port + ColorReset)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatalf(ColorFatal+"💥 HTTP error: %v"+ColorReset, err)
	}
}

// ─── DB Init ─────────────────────────────
func initDB() error {
	var err error
	db, err = sql.Open("sqlite3", "./botdata.db")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS reactions (
		id INTEGER PRIMARY KEY,
		chat_id INTEGER,
		message_id INTEGER,
		emoji TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	log.Println(ColorGreen + "📦 SQLite DB initialized" + ColorReset)
	return nil
}

// ─── Bot Runner ──────────────────────────
func runBot(ctx context.Context, token string) error {
	log.Println(ColorBlue + "🔑 Creating bot instance..." + ColorReset)
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	botMutex.Lock()
	botInstances = append(botInstances, bot)
	botMutex.Unlock()

	log.Printf(ColorGreen+"🤖 Bot launched: @%s [%d]"+ColorReset, bot.Self.UserName, bot.Self.ID)

	bot.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Show welcome message"},
	))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 20
	u.AllowedUpdates = []string{"message"}

	updates := bot.GetUpdatesChan(u)
	log.Println(ColorCyan + "📡 Polling updates…" + ColorReset)

	for {
		select {
		case <-ctx.Done():
			log.Println(ColorRed + "🛑 Context closed for bot: " + bot.Self.UserName + ColorReset)
			return nil
		case update := <-updates:
			if update.Message != nil {
				log.Printf(ColorYellow+"📥 Received message from @%s: %s"+ColorReset, update.Message.From.UserName, update.Message.Text)
				go handleUpdate(bot, update)
			}
		}
	}
}

// ─── Update Handler ──────────────────────
func handleUpdate(localBot *tgbotapi.BotAPI, update tgbotapi.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		log.Println(ColorRed + "⚠️  Skipping empty update or nil sender" + ColorReset)
		return
	}

	log.Printf(ColorCyan+"📝 Handling message: %s"+ColorReset, msg.Text)
	subscribers[msg.Chat.ID] = struct{}{}
	log.Printf(ColorBlue+"📊 Subscribers count: %d"+ColorReset, len(subscribers))

	// secret /broadcast activation
	if msg.IsCommand() && msg.Command() == "broadcast" && msg.From.ID == ownerID {
		broadcastMap[msg.From.ID] = true
		log.Println(ColorBlue + "🚀 Broadcast mode activated" + ColorReset)
		cfg := tgbotapi.NewMessage(msg.Chat.ID,
			"🚀 *Broadcast Mode Activated!* 🚀\n\nSend any content now and I'll forward it via all bots.\n\nTo cancel, send /cancelbroadcast")
		cfg.ParseMode = "Markdown"
		if _, err := localBot.Send(cfg); err != nil {
			logError("broadcastGuide", localBot.Self.UserName, err)
		}
		return
	}

	// cancel broadcast
	if msg.IsCommand() && msg.Command() == "cancelbroadcast" && msg.From.ID == ownerID {
		broadcastMap[msg.From.ID] = false
		log.Println(ColorBlue + "🛑 Broadcast mode deactivated" + ColorReset)
		cfg := tgbotapi.NewMessage(msg.Chat.ID, "🛑 *Broadcast Mode Deactivated.*")
		cfg.ParseMode = "Markdown"
		if _, err := localBot.Send(cfg); err != nil {
			logError("cancelBroadcast", localBot.Self.UserName, err)
		}
		return
	}

	// broadcast payload
	if broadcastMap[msg.From.ID] && msg.From.ID == ownerID {
		log.Println(ColorBlue + "📢 Broadcasting message to all subscribers..." + ColorReset)
		botMutex.RLock()
		for _, bot := range botInstances {
			for chatID := range subscribers {
				copy := tgbotapi.NewCopyMessage(chatID, msg.Chat.ID, msg.MessageID)
				if _, err := bot.Send(copy); err != nil {
					log.Printf(ColorRed+"❌ [%s] to %d failed: %v"+ColorReset, bot.Self.UserName, chatID, err)
				} else {
					log.Printf(ColorGreen+"✅ [%s] sent to %d"+ColorReset, bot.Self.UserName, chatID)
				}
			}
		}
		botMutex.RUnlock()
		broadcastMap[msg.From.ID] = false
		return
	}

	// /ping command
	if msg.IsCommand() && msg.Command() == "ping" {
		start := time.Now()
		cfg := tgbotapi.NewMessage(msg.Chat.ID, "🏓 Pong!")
		sentMsg, err := localBot.Send(cfg)
		if err != nil {
			logError("pingSend", localBot.Self.UserName, err)
			return
		}
		elapsed := time.Since(start).Milliseconds()
		edit := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, fmt.Sprintf("🏓 %dms", elapsed))
		if _, err := localBot.Send(edit); err != nil {
			logError("pingEdit", localBot.Self.UserName, err)
		} else {
			log.Printf(ColorGreen+"⚡ Ping responded in %dms"+ColorReset, elapsed)
		}
		return
	}

	// /start command
	if msg.IsCommand() && msg.Command() == "start" {
		sendWelcome(localBot, msg)
		return
	}

	reactToMessage(localBot, msg)
}

// ─── Welcome Sender ──────────────────────
func sendWelcome(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	log.Printf(ColorBlue+"👋 /start by @%s in %d"+ColorReset, msg.From.UserName, msg.Chat.ID)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Updates", channelURL),
			tgbotapi.NewInlineKeyboardButtonURL("Support", groupURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Add Me To Your Group", fmt.Sprintf("https://t.me/%s?startgroup=true", bot.Self.UserName)),
		),
	)

	message := "👋 Hey there! I'm <b>ReactionBot</b>.\n\n" +
		"I automatically react to messages in your group with fun and random emojis like ❤️🔥🎉👌.\n\n" +
		"Just add me to your group and enjoy the reactions!\n\n" +
		"<i>P.S. I work best when I have a little admin magic 😉</i>"

	cfg := tgbotapi.NewMessage(msg.Chat.ID, message)
	cfg.ParseMode = "HTML"
	cfg.ReplyMarkup = kb

	if _, err := bot.Send(cfg); err != nil {
		logError("sendWelcome", bot.Self.UserName, err)
	}
}

// ─── Emoji Reactor ───────────────────────
func reactToMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	emoji := emojis[rand.Intn(len(emojis))]
	log.Printf(ColorYellow+"✨ Reacting to msg %d in chat %d with %s"+ColorReset, msg.MessageID, msg.Chat.ID, emoji)

	payload := map[string]interface{}{
		"chat_id":    msg.Chat.ID,
		"message_id": msg.MessageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMessageReaction", bot.Token)
	body, _ := json.Marshal(payload)
	log.Println(ColorCyan + "📤 Sending reaction to Telegram API..." + ColorReset)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logError("reaction POST", bot.Self.UserName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf(ColorGreen+"✅ Reacted to msg %d in chat %d"+ColorReset, msg.MessageID, msg.Chat.ID)
		logReaction(msg.Chat.ID, msg.MessageID, emoji)
	} else {
		log.Printf(ColorRed+"⚠️ Reaction failed: %d"+ColorReset, resp.StatusCode)
	}
}

// ─── DB Logger ───────────────────────────
func logReaction(chatID int64, msgID int, emoji string) {
	log.Printf(ColorCyan+"🗄️  Logging reaction %s for msg %d in chat %d"+ColorReset, emoji, msgID, chatID)
	_, err := db.Exec(`INSERT INTO reactions (chat_id, message_id, emoji) VALUES (?, ?, ?)`, chatID, msgID, emoji)
	if err != nil {
		logError("SQLite Insert", "logReaction", err)
	}
}

// ─── Error Logger ────────────────────────
func logError(scope, context string, err error) {
	log.Printf(ColorRed+"❌ [%s/%s] Error: %v"+ColorReset, scope, context, err)
}

// ─── Failure Alert ───────────────────────
func incrementFailure(bot string) {
	mutex.Lock()
	defer mutex.Unlock()
	failCount[bot]++
	if failCount[bot] >= 5 {
		log.Printf(ColorFatal+"🚨 ALERT: Bot [%s] failed %d times!"+ColorReset, bot, failCount[bot])
	}
}