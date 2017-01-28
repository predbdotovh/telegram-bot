package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/telegram-bot-api.v4"
)

type configuration struct {
	Token          string
	WebhookHost    string
	WebhookRoot    string
	LocalListen    string
	SphinxDatabase string
}

func main() {
	log.Print("Hello")

	log.Print("Open file")
	file, err := os.Open("conf/telegram-bot.config.json")
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Decode file")
	decoder := json.NewDecoder(file)
	conf := configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Init bot API")
	bot, err := tgbotapi.NewBotAPI(conf.Token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	_, err = bot.SetWebhook(tgbotapi.NewWebhook(conf.WebhookHost + conf.WebhookRoot + bot.Token))
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("mysql", conf.SphinxDatabase)
	if err != nil {
		log.Fatal(err)
	}

	log.Print("Listen for webhook")
	updates := bot.ListenForWebhook(conf.WebhookRoot + bot.Token)
	go http.ListenAndServe(conf.LocalListen, nil)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, db, update.Message)
		} else if update.EditedMessage != nil {
			handleMessage(bot, db, update.EditedMessage)
		} else if update.InlineQuery != nil {
			handleInline(bot, db, update.InlineQuery)
		} else {
			log.Printf("%+v\n", update)
		}
	}
}

type sphinxRow struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Team  string  `json:"team"`
	Cat   string  `json:"cat"`
	Genre string  `json:"genre"`
	URL   string  `json:"url"`
	Size  float64 `json:"size"`
	Files int     `json:"files"`
	PreAt int64   `json:"preAt"`
	pre   time.Time
	//Nuke  *nuke   `json:"nuke"`
}

func (s sphinxRow) short() string {
	return fmt.Sprintf("%s %s", s.Name, s.pre.String())
}

var replacer = strings.NewReplacer("(", "\\(", ")", "\\)")

func querySphinx(db *sql.DB, q string, max int) ([]sphinxRow, error) {
	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = db.Query("SELECT id, name, team, cat, genre, url, size, files, pre_at FROM pre_plain,pre_rt ORDER BY id DESC LIMIT " + strconv.Itoa(max) + " OPTION reverse_scan = 1")
	} else {
		rows, err = db.Query("SELECT id, name, team, cat, genre, url, size, files, pre_at FROM pre_plain,pre_rt WHERE MATCH(?) ORDER BY id DESC LIMIT "+strconv.Itoa(max)+" OPTION reverse_scan = 1", replacer.Replace(q))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]sphinxRow, 0)
	for rows.Next() {
		var r sphinxRow
		err = rows.Scan(&r.ID, &r.Name, &r.Team, &r.Cat, &r.Genre, &r.URL, &r.Size, &r.Files, &r.PreAt)
		if err != nil {
			log.Print(err)
			continue
		}

		r.pre = time.Unix(r.PreAt, 0)

		res = append(res, r)
	}

	return res, nil
}

const inlineMaxRes = 1

func handleInline(bot *tgbotapi.BotAPI, db *sql.DB, iq *tgbotapi.InlineQuery) {
	log.Printf("%+v\n", iq)

	rows, err := querySphinx(db, iq.Query, inlineMaxRes)
	if err != nil {
		log.Print(err)
		return
	}

	res := make([]interface{}, 0)
	for i, row := range rows {
		res = append(res, tgbotapi.NewInlineQueryResultArticle(string(i), row.Name, row.short()))
	}
	answer := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		Results:       res,
	}

	response, err := bot.AnswerInlineQuery(answer)
	log.Print(response)
	if err != nil {
		log.Print(err)
	}
}

const directMaxRes = 5

func handleMessage(bot *tgbotapi.BotAPI, db *sql.DB, m *tgbotapi.Message) {
	log.Printf("%+v\n", m)

	if len(m.Text) == 0 {
		return
	}

	if m.IsCommand() {
		handleCommand(bot, db, m, m.Command(), m.CommandArguments())
		return
	}

	// Don't bother with group messages
	if !m.Chat.IsPrivate() {
		return
	}

	rows, err := querySphinx(db, m.Text, directMaxRes)
	if err != nil {
		log.Print(err)
		return
	}

	for _, row := range rows {
		bot.Send(tgbotapi.NewMessage(m.Chat.ID, row.short()))
	}
}

func handleCommand(bot *tgbotapi.BotAPI, db *sql.DB, m *tgbotapi.Message, command, args string) {
	if !(m.Chat.IsPrivate() || strings.HasPrefix(m.Text, "/"+command+"@"+bot.Self.UserName)) {
		return
	}

	switch command {
	case "start":
		handleCommandStart(bot, m)
	case "help":
		handleCommandHelp(bot, m)
	case "ping":
		handleCommandPing(bot, m)
	case "query":
		handleCommandQuery(bot, db, m, args)
	default:
		handleCommandUnknown(bot, m)
	}
}

const startContent = `Hello !
This bot has inline mode activated, feel free to query me :
@PredbBot <query>
Type /help for available commands.`

func handleCommandStart(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	// /start shouldn't happen outside of private
	if m.Chat.IsPrivate() {
		bot.Send(tgbotapi.NewMessage(m.Chat.ID, startContent))
	}
}

const helpContent = `/ping : Check if I'm still alive
/query <string> : Query for release name at predb.ovh`

func handleCommandHelp(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	bot.Send(tgbotapi.NewMessage(m.Chat.ID, helpContent))
}

func handleCommandPing(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	bot.Send(tgbotapi.NewMessage(m.Chat.ID, "Pong"))
}

const queryMaxRes = 3

func handleCommandQuery(bot *tgbotapi.BotAPI, db *sql.DB, m *tgbotapi.Message, args string) {
	rows, err := querySphinx(db, args, queryMaxRes)
	if err != nil {
		log.Print(err)
		return
	}

	for _, row := range rows {
		bot.Send(tgbotapi.NewMessage(m.Chat.ID, row.short()))
	}
}

func handleCommandUnknown(bot *tgbotapi.BotAPI, m *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(m.Chat.ID, "I didn't understand that. List available commands with /help")
	bot.Send(msg)
}
