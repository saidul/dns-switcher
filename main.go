package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/telegram"
)

type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

func sendTelegramMessage(botToken, chatID, message string) error {
	notifier := notify.New()
	telegramService := telegram.New(botToken)
	telegramService.AddReceivers(chatID)
	notifier.UseServices(telegramService)

	return notifier.Send(
		"DNS Update Notification",
		message,
	)
}

func checkIP(ip string, domain string) bool {
	url := fmt.Sprintf("http://%s", ip)
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	return true
}

func updateDNSRecord(apiToken, zoneID, recordID, ip, domain string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	record := DNSRecord{
		Type:    "A",
		Name:    domain,
		Content: ip,
		TTL:     120,
	}
	jsonData, err := json.Marshal(record)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update DNS record, status code: %d", resp.StatusCode)
	}

	return nil
}

func monitorIPs(apiToken, zoneID string, recordIDs []string, ips []string, domain string) {
	currentIndex := 0
	for {
		currentIP := ips[currentIndex]
		if !checkIP(currentIP, domain) {
			log.Printf("IP %s is down, checking next IP...\n", currentIP)
			nextIndex := (currentIndex + 1) % len(ips)
			nextIP := ips[nextIndex]
			if checkIP(nextIP, domain) {
				log.Printf("Next IP %s is online, updating DNS records...\n", nextIP)
				for _, recordID := range recordIDs {
					err := updateDNSRecord(apiToken, zoneID, recordID, nextIP, domain)
					if err != nil {
						log.Printf("Error updating DNS record: %v\n", err)
					} else {
						log.Printf("DNS record updated to: %s\n", nextIP)
					}
				}
				currentIndex = nextIndex
			} else {
				log.Printf("Next IP %s is also down, not updating DNS records.\n", nextIP)
			}
		} else {
			log.Printf("IP %s is online, no action needed.\n", currentIP)
		}
		time.Sleep(30 * time.Second)
	}
}

func main() {
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	zoneID := os.Getenv("CLOUDFLARE_ZONE_ID")
	recordIDs := strings.Split(os.Getenv("CLOUDFLARE_RECORD_IDS"), ",")
	ips := strings.Split(os.Getenv("MONITOR_IPS"), ",")
	domain := os.Getenv("DOMAIN")
	telegramBotToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatID := os.Getenv("TELEGRAM_CHAT_ID")

	if apiToken == "" || zoneID == "" || len(recordIDs) == 0 || len(ips) == 0 || domain == "" {
		log.Fatal("Environment variables CLOUDFLARE_API_TOKEN, CLOUDFLARE_ZONE_ID, CLOUDFLARE_RECORD_IDS, MONITOR_IPS, and DOMAIN must be set")
		return
	}

	go monitorIPs(apiToken, zoneID, recordIDs, ips, domain)
	select {} // Block forever
}

// docker run -e CLOUDFLARE_API_TOKEN=your_api_token \
//            -e CLOUDFLARE_ZONE_ID=your_zone_id \
//            -e CLOUDFLARE_RECORD_IDS=record_id1,record_id2 \
//            -e MONITOR_IPS=192.0.2.1,192.0.2.2 \
//            -e DOMAIN=example.com \
//            -e TELEGRAM_BOT_TOKEN=your_telegram_bot_token \
//            -e TELEGRAM_CHAT_ID=your_telegram_chat_id \
//            cloudflare-updater
