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

// ─── Colors ───────────────────────────
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[1;31m"
	ColorGreen  = "\033[1;32m"
	ColorYellow = "\033[1;33m"
	ColorBlue   = "\033[1;34m"
	ColorCyan   = "\033[0;36m"
	ColorFatal  = "\033[1;41m"
)

// ─── Sakura Images ───────────────────────
var SAKURA_IMAGES = []string{
	"https://ik.imagekit.io/asadofc/Images1.png",
	"https://ik.imagekit.io/asadofc/Images2.png",
	"https://ik.imagekit.io/asadofc/Images3.png",
	"https://ik.imagekit.io/asadofc/Images4.png",
	"https://ik.imagekit.io/asadofc/Images5.png",
	"https://ik.imagekit.io/asadofc/Images6.png",
	"https://ik.imagekit.io/asadofc/Images7.png",
	"https://ik.imagekit.io/asadofc/Images8.png",
	"https://ik.imagekit.io/asadofc/Images9.png",
	"https://ik.imagekit.io/asadofc/Images10.png",
	"https://ik.imagekit.io/asadofc/Images11.png",
	"https://ik.imagekit.io/asadofc/Images12.png",
	"https://ik.imagekit.io/asadofc/Images13.png",
	"https://ik.imagekit.io/asadofc/Images14.png",
	"https://ik.imagekit.io/asadofc/Images15.png",
	"https://ik.imagekit.io/asadofc/Images16.png",
	"https://ik.imagekit.io/asadofc/Images17.png",
	"https://ik.imagekit.io/asadofc/Images18.png",
	"https://ik.imagekit.io/asadofc/Images19.png",
	"https://ik.imagekit.io/asadofc/Images20.png",
	"https://ik.imagekit.io/asadofc/Images21.png",
	"https://ik.imagekit.io/asadofc/Images22.png",
	"https://ik.imagekit.io/asadofc/Images23.png",
	"https://ik.imagekit.io/asadofc/Images24.png",
	"https://ik.imagekit.io/asadofc/Images25.png",
	"https://ik.imagekit.io/asadofc/Images26.png",
	"https://ik.imagekit.io/asadofc/Images27.png",
	"https://ik.imagekit.io/asadofc/Images28.png",
	"https://ik.imagekit.io/asadofc/Images29.png",
	"https://ik.imagekit.io/asadofc/Images30.png",
	"https://ik.imagekit.io/asadofc/Images31.png",
	"https://ik.imagekit.io/asadofc/Images32.png",
	"https://ik.imagekit.io/asadofc/Images33.png",
	"https://ik.imagekit.io/asadofc/Images34.png",
	"https://ik.imagekit.io/asadofc/Images35.png",
	"https://ik.imagekit.io/asadofc/Images36.png",
	"https://ik.imagekit.io/asadofc/Images37.png",
	"https://ik.imagekit.io/asadofc/Images38.png",
	"https://ik.imagekit.io/asadofc/Images39.png",
	"https://ik.imagekit.io/asadofc/Images40.png",
}

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

	// Group reaction control
	groupReactions = make(map[int64]bool)     // chatID -> reactions enabled/disabled
	reactionMutex  sync.RWMutex              // protects groupReactions

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

// ─── Chat Type Checker ───────────────────
func isGroup(chat *tgbotapi.Chat) bool {
	return chat.Type == "group" || chat.Type == "supergroup"
}

// ─── Group Reaction Status ───────────────
func areReactionsEnabled(chatID int64) bool {
	reactionMutex.RLock()
	defer reactionMutex.RUnlock()
	enabled, exists := groupReactions[chatID]
	// Default is enabled for private chats and groups that haven't set preference
	if !exists {
		return true
	}
	return enabled
}

func setReactionsEnabled(chatID int64, enabled bool) {
	reactionMutex.Lock()
	defer reactionMutex.Unlock()
	groupReactions[chatID] = enabled
	log.Printf(ColorBlue+"🎛️  Reactions %s for chat %d"+ColorReset, 
		map[bool]string{true: "ENABLED", false: "DISABLED"}[enabled], chatID)
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
		tgbotapi.BotCommand{Command: "begin", Description: "Start reactions in group"},
		tgbotapi.BotCommand{Command: "end", Description: "Stop reactions in group"},
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
		mutex.Lock()
		broadcastMap[ownerID] = true
		mutex.Unlock()
		log.Printf(ColorBlue+"🚀 Broadcast mode activated by @%s via bot %s"+ColorReset, msg.From.UserName, localBot.Self.UserName)
		cfg := tgbotapi.NewMessage(msg.Chat.ID,
			"🚀 *Broadcast Mode Activated!* 🚀\n\nSend any content now and I'll forward it via ALL bots to all subscribers.\n\nTo cancel, send /cancelbroadcast")
		cfg.ParseMode = "Markdown"
		if _, err := localBot.Send(cfg); err != nil {
			logError("broadcastGuide", localBot.Self.UserName, err)
		}
		return
	}

	// cancel broadcast
	if msg.IsCommand() && msg.Command() == "cancelbroadcast" && msg.From.ID == ownerID {
		mutex.Lock()
		broadcastMap[ownerID] = false
		mutex.Unlock()
		log.Printf(ColorBlue+"🛑 Broadcast mode deactivated by @%s via bot %s"+ColorReset, msg.From.UserName, localBot.Self.UserName)
		cfg := tgbotapi.NewMessage(msg.Chat.ID, "🛑 *Broadcast Mode Deactivated.*")
		cfg.ParseMode = "Markdown"
		if _, err := localBot.Send(cfg); err != nil {
			logError("cancelBroadcast", localBot.Self.UserName, err)
		}
		return
	}

	// broadcast payload - check if owner is in broadcast mode
	mutex.Lock()
	isBroadcasting := broadcastMap[ownerID]
	mutex.Unlock()
	
	if isBroadcasting && msg.From.ID == ownerID {
		log.Printf(ColorBlue+"📢 Broadcasting message from @%s via bot %s to all subscribers..."+ColorReset, msg.From.UserName, localBot.Self.UserName)
		
		var successCount, failCount int
		botMutex.RLock()
		for _, bot := range botInstances {
			for chatID := range subscribers {
				copy := tgbotapi.NewCopyMessage(chatID, msg.Chat.ID, msg.MessageID)
				if _, err := bot.Send(copy); err != nil {
					log.Printf(ColorRed+"❌ [%s] to %d failed: %v"+ColorReset, bot.Self.UserName, chatID, err)
					failCount++
				} else {
					log.Printf(ColorGreen+"✅ [%s] sent to %d"+ColorReset, bot.Self.UserName, chatID)
					successCount++
				}
			}
		}
		botMutex.RUnlock()
		
		// Send broadcast summary
		summary := fmt.Sprintf("📊 *Broadcast Complete!*\n\n✅ Successful: %d\n❌ Failed: %d\n🤖 Total Bots: %d\n👥 Total Subscribers: %d", 
			successCount, failCount, len(botInstances), len(subscribers))
		cfg := tgbotapi.NewMessage(msg.Chat.ID, summary)
		cfg.ParseMode = "Markdown"
		if _, err := localBot.Send(cfg); err != nil {
			logError("broadcastSummary", localBot.Self.UserName, err)
		}
		
		// Auto-deactivate broadcast mode after sending
		mutex.Lock()
		broadcastMap[ownerID] = false
		mutex.Unlock()
		log.Println(ColorBlue + "🛑 Broadcast mode auto-deactivated after sending" + ColorReset)
		return
	}

	// /begin command - only for groups
	if msg.IsCommand() && msg.Command() == "begin" {
		if !isGroup(msg.Chat) {
			cfg := tgbotapi.NewMessage(msg.Chat.ID, "❌ This command only works in groups!")
			if _, err := localBot.Send(cfg); err != nil {
				logError("beginPrivateError", localBot.Self.UserName, err)
			}
			return
		}
		
		setReactionsEnabled(msg.Chat.ID, true)
		
		// React to the command first
		go reactToMessage(localBot, msg)
		
		// Send confirmation with random image
		randomImage := SAKURA_IMAGES[rand.Intn(len(SAKURA_IMAGES))]
		
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("Updates", channelURL),
				tgbotapi.NewInlineKeyboardButtonURL("Support", groupURL),
			),
		)
		
		message := "💖 Reactions Started! 💖\n\n" +
			"I'm now actively reacting to messages in this group with fun emojis! ✨\n\n" +
			"Use /end to stop reactions anytime.\n\n" +
			"<i>Let's make this chat more lively! 💞</i>"
		
		photo := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileURL(randomImage))
		photo.Caption = message
		photo.ParseMode = "HTML"
		photo.ReplyMarkup = kb
		
		if _, err := localBot.Send(photo); err != nil {
			log.Printf(ColorRed+"❌ Failed to send begin photo, falling back to text: %v"+ColorReset, err)
			cfg := tgbotapi.NewMessage(msg.Chat.ID, message)
			cfg.ParseMode = "HTML"
			cfg.ReplyMarkup = kb
			if _, err := localBot.Send(cfg); err != nil {
				logError("beginFallback", localBot.Self.UserName, err)
			}
		}
		
		log.Printf(ColorGreen+"✅ Reactions enabled for group %d"+ColorReset, msg.Chat.ID)
		return
	}

	// /end command - only for groups
	if msg.IsCommand() && msg.Command() == "end" {
		if !isGroup(msg.Chat) {
			cfg := tgbotapi.NewMessage(msg.Chat.ID, "❌ This command only works in groups!")
			if _, err := localBot.Send(cfg); err != nil {
				logError("endPrivateError", localBot.Self.UserName, err)
			}
			return
		}
		
		setReactionsEnabled(msg.Chat.ID, false)
		
		cfg := tgbotapi.NewMessage(msg.Chat.ID, 
			"👋 Reactions Stopped! 👋\n\n"+
			"I've stopped reacting to messages in this group.\n\n"+
			"Use /begin to start reactions again! ✨")
		cfg.ParseMode = "HTML"
		
		if _, err := localBot.Send(cfg); err != nil {
			logError("endCommand", localBot.Self.UserName, err)
		}
		
		log.Printf(ColorYellow+"🛑 Reactions disabled for group %d"+ColorReset, msg.Chat.ID)
		return
	}

	// /start command - different behavior for groups vs private
	if msg.IsCommand() && msg.Command() == "start" {
		if isGroup(msg.Chat) {
			// Group start command with image
			sendGroupWelcome(localBot, msg)
		} else {
			// Private start command (original behavior)
			go reactToMessage(localBot, msg)
			sendWelcome(localBot, msg)
		}
		return
	}

	// /ping command - react first, then respond
	if msg.IsCommand() && msg.Command() == "ping" {
		// Only react if reactions are enabled or in private chat
		if !isGroup(msg.Chat) || areReactionsEnabled(msg.Chat.ID) {
			go reactToMessage(localBot, msg)
		}
		
		log.Println(ColorBlue + "🏓 /ping command received" + ColorReset)
		start := time.Now()

		// Create initial message - reply in groups, simple message in private
		var cfg tgbotapi.MessageConfig
		if isGroup(msg.Chat) {
			// In groups: reply to the original message
			cfg = tgbotapi.NewMessage(msg.Chat.ID, "🛰️ Pinging...")
			cfg.ReplyToMessageID = msg.MessageID
		} else {
			// In private: send simple message without reply
			cfg = tgbotapi.NewMessage(msg.Chat.ID, "🛰️ Pinging...")
		}

		sentMsg, err := localBot.Send(cfg)
		if err != nil {
			logError("pingSend", localBot.Self.UserName, err)
			return
		}

		elapsed := float64(time.Since(start).Microseconds()) / 1000 // ms
		latency := fmt.Sprintf("%.2fms", elapsed)

		text := fmt.Sprintf("🏓 [Pong\\!](https://t.me/SoulMeetsHQ) %s", escapeMarkdownV2(latency))
		edit := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, text)
		edit.ParseMode = "MarkdownV2"
		edit.DisableWebPagePreview = true

		if _, err := localBot.Send(edit); err != nil {
			logError("pingEdit", localBot.Self.UserName, err)
		} else {
			log.Printf(ColorGreen+"⚡ Ping responded in %s"+ColorReset, latency)
		}
		return
	}

	// React to regular messages based on chat type and settings
	if !isGroup(msg.Chat) {
		// Always react in private chats
		reactToMessage(localBot, msg)
	} else if areReactionsEnabled(msg.Chat.ID) {
		// Only react in groups if reactions are enabled
		reactToMessage(localBot, msg)
	} else {
		log.Printf(ColorYellow+"⏸️  Skipping reaction for group %d (reactions disabled)"+ColorReset, msg.Chat.ID)
	}
}

// ─── Escape helper ───────────────────────
func escapeMarkdownV2(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

// ─── Group Welcome Sender ────────────────
func sendGroupWelcome(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	log.Printf(ColorBlue+"👋 Group /start by @%s in %d"+ColorReset, msg.From.UserName, msg.Chat.ID)

	// Select a random Sakura image
	randomImage := SAKURA_IMAGES[rand.Intn(len(SAKURA_IMAGES))]
	log.Printf(ColorCyan+"🌸 Selected random Sakura image for group: %s"+ColorReset, randomImage)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Updates", channelURL),
			tgbotapi.NewInlineKeyboardButtonURL("Support", groupURL),
		),
	)

	message := "❤️ Hello Everyone! I'm <b>ReactionBot</b>!\n\n" +
		"I'm here to make your group more fun with automatic emoji reactions! ✨\n\n" +
		"📋 <b>Group Commands:</b>\n" +
		"• /begin - Start reactions\n" +
		"• /end - Stop reactions\n" +
		"• /ping - Check my response time\n\n" +
		"<i>Ready to bring some life to your conversations! 💞</i>"

	// Send photo with caption
	photo := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileURL(randomImage))
	photo.Caption = message
	photo.ParseMode = "HTML"
	photo.ReplyMarkup = kb

	if _, err := bot.Send(photo); err != nil {
		log.Printf(ColorRed+"❌ Failed to send group photo, falling back to text: %v"+ColorReset, err)
		// Fallback to text message if photo fails
		cfg := tgbotapi.NewMessage(msg.Chat.ID, message)
		cfg.ParseMode = "HTML"
		cfg.ReplyMarkup = kb
		if _, err := bot.Send(cfg); err != nil {
			logError("sendGroupWelcome fallback", bot.Self.UserName, err)
		}
	} else {
		log.Printf(ColorGreen+"✅ Group welcome message with image sent successfully"+ColorReset)
	}
}

// ─── Private Welcome Sender ──────────────
func sendWelcome(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	log.Printf(ColorBlue+"👋 Private /start by @%s in %d"+ColorReset, msg.From.UserName, msg.Chat.ID)

	// Select a random Sakura image
	randomImage := SAKURA_IMAGES[rand.Intn(len(SAKURA_IMAGES))]
	log.Printf(ColorCyan+"🌸 Selected random Sakura image for private: %s"+ColorReset, randomImage)

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

	// Send photo with caption
	photo := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileURL(randomImage))
	photo.Caption = message
	photo.ParseMode = "HTML"
	photo.ReplyMarkup = kb

	if _, err := bot.Send(photo); err != nil {
		log.Printf(ColorRed+"❌ Failed to send private photo, falling back to text: %v"+ColorReset, err)
		// Fallback to text message if photo fails
		cfg := tgbotapi.NewMessage(msg.Chat.ID, message)
		cfg.ParseMode = "HTML"
		cfg.ReplyMarkup = kb
		if _, err := bot.Send(cfg); err != nil {
			logError("sendWelcome fallback", bot.Self.UserName, err)
		}
	} else {
		log.Printf(ColorGreen+"✅ Private welcome message with image sent successfully"+ColorReset)
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