package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/telegram-bot-api.v4"
)

type configuration struct {
	Token          string
	WebhookHost    string
	WebhookRoot    string
	LocalListen    string
	SphinxDatabase string
}

type apiResponse struct {
	Status  string     `json:"status"`
	Message string     `json:"message"`
	Data    apiRowData `json:"data"`
}

type apiRowData struct {
	RowCount int         `json:"rowCount"`
	Rows     []sphinxRow `json:"rows"`
	Offset   int         `json:"offset"`
	ReqCount int         `json:"reqCount"`
	Total    int         `json:"total"`
	Time     float64     `json:"time"`
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
}

func (s sphinxRow) preAt() time.Time {
	return time.Unix(s.PreAt, 0)
}

func (s sphinxRow) short() string {
	return fmt.Sprintf("%s %s", s.Name, s.preAt().String())
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

	// bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	_, err = bot.SetWebhook(tgbotapi.NewWebhook(conf.WebhookHost + conf.WebhookRoot + bot.Token))
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	log.Print("Listen for webhook")
	updates := bot.ListenForWebhook(conf.WebhookRoot + bot.Token)
	go http.ListenAndServe(conf.LocalListen, nil)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, client, update.Message)
		} else if update.EditedMessage != nil {
			handleMessage(bot, client, update.EditedMessage)
		} else if update.InlineQuery != nil {
			handleInline(bot, client, update.InlineQuery)
		} else {
			log.Printf("%+v\n", update)
		}
	}
}

var replacer = strings.NewReplacer("(", "\\(", ")", "\\)")

func querySphinx(client *http.Client, q string, max int) ([]sphinxRow, error) {
	resp, err := client.Get(fmt.Sprintf("https://predb.ovh/api/v1/?q=%s&count=%d", url.QueryEscape(q), max))
	if err != nil {
		return nil, err
	}

	var api apiResponse

	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&api)
	if err != nil {
		return nil, err
	}

	if api.Status != "success" {
		log.Println(resp.Body)
		return nil, errors.New("Internal error")
	}

	return api.Data.Rows, nil
}

const inlineMaxRes = 1

func handleInline(bot *tgbotapi.BotAPI, client *http.Client, iq *tgbotapi.InlineQuery) {
	log.Printf("i> %s [%d] : %s\n", iq.From, iq.From.ID, iq.Query)

	rows, err := querySphinx(client, iq.Query, inlineMaxRes)
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

	log.Printf("i< Send back %d results\n", len(res))
	_, err = bot.AnswerInlineQuery(answer)
	if err != nil {
		log.Print(err)
	}
}

const directMaxRes = 5

func handleMessage(bot *tgbotapi.BotAPI, client *http.Client, m *tgbotapi.Message) {
	log.Printf("m> %s [%d] : %s\n", m.From, m.From.ID, m.Text)

	if len(m.Text) == 0 {
		return
	}

	if m.IsCommand() {
		handleCommand(bot, client, m, m.Command(), m.CommandArguments())
		return
	}

	// Don't bother with group messages
	if !m.Chat.IsPrivate() {
		return
	}

	rows, err := querySphinx(client, m.Text, directMaxRes)
	if err != nil {
		log.Print(err)
		return
	}

	log.Printf("m< Send back %d results\n", len(rows))
	for _, row := range rows {
		bot.Send(tgbotapi.NewMessage(m.Chat.ID, row.short()))
	}
}

func handleCommand(bot *tgbotapi.BotAPI, client *http.Client, m *tgbotapi.Message, command, args string) {
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
		handleCommandQuery(bot, client, m, args)
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

func handleCommandQuery(bot *tgbotapi.BotAPI, client *http.Client, m *tgbotapi.Message, args string) {
	rows, err := querySphinx(client, args, queryMaxRes)
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
