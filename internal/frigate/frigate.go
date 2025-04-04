package frigate

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oldtyt/frigate-telegram/internal/config"
	"github.com/oldtyt/frigate-telegram/internal/log"
	"github.com/oldtyt/frigate-telegram/internal/redis"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type EventsStruct []struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            float64     `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	// SubLabel           []any       `json:"sub_label"`
	Thumbnail string      `json:"thumbnail"`
	TopScore  interface{} `json:"top_score"`
	Zones     []any       `json:"zones"`
}

type EventStruct struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            float64     `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	// SubLabel           []any       `json:"sub_label"`
	Thumbnail string      `json:"thumbnail"`
	TopScore  interface{} `json:"top_score"`
	Zones     []any       `json:"zones"`
}

var Events EventsStruct
var Event EventStruct

func NormalizeTagText(text string) string {
	var alphabetCheck = regexp.MustCompile(`^[A-Za-z]+$`)
	var NormalizedText []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		wordString := fmt.Sprintf("%c", runes[i])
		if _, err := strconv.Atoi(wordString); err == nil {
			NormalizedText = append(NormalizedText, wordString)
		}
		if alphabetCheck.MatchString(wordString) {
			NormalizedText = append(NormalizedText, wordString)
		}
	}
	return strings.Join(NormalizedText, "")
}

func GetTagList(Tags []any) []string {
	var my_tags []string
	for _, zone := range Tags {
		if zone != nil {
			my_tags = append(my_tags, NormalizeTagText(zone.(string)))
		}
	}
	return my_tags
}

func ErrorSend(TextError string, bot *tgbotapi.BotAPI, EventID string) {
	conf := config.New()
	TextError += "\nEventID: " + EventID
	_, err := bot.Send(tgbotapi.NewMessage(conf.TelegramChatID, TextError))
	if err != nil {
		log.Error.Println(err.Error())
	}
	log.Error.Fatalln(TextError)
}

func SaveThumbnail(EventID string, Thumbnail string, bot *tgbotapi.BotAPI) string {
	// Decode string Thumbnail base64
	dec, err := base64.StdEncoding.DecodeString(Thumbnail)
	if err != nil {
		ErrorSend("Error when base64 string decode: "+err.Error(), bot, EventID)
	}

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".jpg"
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), bot, EventID)
	}
	defer f.Close()
	if _, err := f.Write(dec); err != nil {
		ErrorSend("Error when write file: "+err.Error(), bot, EventID)
	}
	if err := f.Sync(); err != nil {
		ErrorSend("Error when sync file: "+err.Error(), bot, EventID)
	}
	return filename
}

func GetEvents(FrigateURL string, bot *tgbotapi.BotAPI, SetBefore bool) EventsStruct {
	conf := config.New()

	FrigateURL = FrigateURL + "?limit=" + strconv.Itoa(conf.FrigateEventLimit)

	if SetBefore {
		timestamp := time.Now().UTC().Unix()
		timestamp = timestamp - int64(conf.EventBeforeSeconds)
		FrigateURL = FrigateURL + "&before=" + strconv.FormatInt(timestamp, 10)
	}

	log.Debug.Println("Geting events from Frigate via URL: " + FrigateURL)

	// Request to Frigate
	resp, err := http.Get(FrigateURL)
	if err != nil {
		ErrorSend("Error get events from Frigate, error: "+err.Error(), bot, "ALL")
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != 200 {
		ErrorSend("Response status != 200, when getting events from Frigate.\nExit.", bot, "ALL")
	}

	// Read data from response
	byteValue, err := io.ReadAll(resp.Body)
	if err != nil {
		ErrorSend("Can't read JSON: "+err.Error(), bot, "ALL")
	}

	// Parse data from JSON to struct
	err1 := json.Unmarshal(byteValue, &Events)
	if err1 != nil {
		ErrorSend("Error unmarshal json: "+err1.Error(), bot, "ALL")
		if e, ok := err.(*json.SyntaxError); ok {
			log.Info.Println("syntax error at byte offset " + strconv.Itoa(int(e.Offset)))
		}
		log.Info.Println("Exit.")
	}

	// Return Events
	return Events
}

func SaveClip(EventID string, bot *tgbotapi.BotAPI) string {
	// Get config
	conf := config.New()

	// Generate clip URL
	ClipURL := conf.FrigateURL + "/api/events/" + EventID + "/clip.mp4"
	log.Debug.Println("Downloading clip from URL: " + ClipURL)

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".mp4"

	// Download clip file
	resp, err := http.Get(ClipURL)
	if err != nil {
		ErrorSend("Error clip download: "+err.Error(), bot, EventID)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		ErrorSend("Return bad status: "+resp.Status, bot, EventID)
	}

	// Read content length if available
	contentLength := resp.ContentLength
	if contentLength == 0 {
		ErrorSend("Received empty clip from server (content length is 0)", bot, EventID)
	}
	log.Debug.Printf("Expected content length: %d bytes", contentLength)

	// Create clip file
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), bot, EventID)
	}
	defer f.Close() // Ensure file is closed even if there's an error

	// Writer the body to file
	bytesWritten, err := io.Copy(f, resp.Body)
	if err != nil {
		ErrorSend("Error clip write: "+err.Error(), bot, EventID)
	}
	log.Debug.Printf("Written %d bytes to %s", bytesWritten, filename)

	// Check if we wrote anything
	if bytesWritten == 0 {
		ErrorSend("No data written to clip file", bot, EventID)
	}
	
	// Ensure file is properly synced to disk
	err = f.Sync()
	if err != nil {
		ErrorSend("Error syncing file to disk: "+err.Error(), bot, EventID)
	}
	
	// Close the file
	err = f.Close()
	if err != nil {
		ErrorSend("Error closing clip file: "+err.Error(), bot, EventID)
	}
	
	// Verify file exists and has content
	fileInfo, err := os.Stat(filename)
	if err != nil {
		ErrorSend("Error verifying clip file: "+err.Error(), bot, EventID)
	}
	
	if fileInfo.Size() == 0 {
		ErrorSend("Clip file is empty after download", bot, EventID)
	}
	
	log.Debug.Printf("Successfully downloaded clip to %s (size: %d bytes)", filename, fileInfo.Size())
	return filename
}

func SendMessageEvent(FrigateEvent EventStruct, bot *tgbotapi.BotAPI) {
	// Get config
	conf := config.New()

	redis.AddNewEvent(FrigateEvent.ID, "InWork", time.Duration(60)*time.Second)

	// Prepare text message
	text := "*Event*\n"
	text += "┣*Camera*\n┗ #" + NormalizeTagText(FrigateEvent.Camera) + "\n"
	text += "┣*Label*\n┗ #" + NormalizeTagText(FrigateEvent.Label) + "\n"
	// if FrigateEvent.SubLabel != nil {
	// 	text += "┣*SubLabel*\n┗ #" + strings.Join(GetTagList(FrigateEvent.SubLabel), ", #") + "\n"
	// }
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s", t_start) + "`\n"
	if FrigateEvent.EndTime == 0 {
		text += "┣*End time*\n┗ `In progess`" + "\n"
	} else {
		t_end := time.Unix(int64(FrigateEvent.EndTime), 0)
		text += fmt.Sprintf("┣*End time*\n┗ `%s", t_end) + "`\n"
	}
	text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
	text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
	text += "┣*Zones*\n┗ #" + strings.Join(GetTagList(FrigateEvent.Zones), ", #") + "\n"
	text += "*URLs*\n"
	text += "┣[Events](" + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",") + ")\n"
	text += "┣[General](" + conf.FrigateExternalURL + ")\n"
	text += "┗[Source clip](" + conf.FrigateExternalURL + "/api/events/" + FrigateEvent.ID + "/clip.mp4)\n"

	// Save thumbnail
	FilePathThumbnail := SaveThumbnail(FrigateEvent.ID, FrigateEvent.Thumbnail, bot)
	
	var medias []interface{}
	MediaThumbnail := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(FilePathThumbnail))
	MediaThumbnail.Caption = text
	MediaThumbnail.ParseMode = tgbotapi.ModeMarkdown
	medias = append(medias, MediaThumbnail)

	// Define FilePathClip outside the if block to make it available later
	var FilePathClip string
	var hasClip bool

	if FrigateEvent.HasClip && FrigateEvent.EndTime != 0 {
		// Save clip
		FilePathClip = SaveClip(FrigateEvent.ID, bot)
		hasClip = true

		videoInfo, err := os.Stat(FilePathClip)
		if err != nil {
			ErrorSend("Error receiving information about the clip file: "+err.Error(), bot, FrigateEvent.ID)
		}

		// Double check file size
		if videoInfo.Size() == 0 {
			log.Error.Printf("Clip file is empty: %s", FilePathClip)
			hasClip = false
		} else if videoInfo.Size() < 52428800 {
			// Telegram don't send large file see for more: https://github.com/OldTyT/frigate-telegram/issues/5
			// Add clip to media group
			log.Debug.Printf("Adding clip to media group: %s (size: %d bytes)", FilePathClip, videoInfo.Size())
			MediaClip := tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(FilePathClip))
			medias = append(medias, MediaClip)
		} else {
			log.Debug.Printf("Clip file size is too large: %d bytes (limit: 52428800)", videoInfo.Size())
		}
	}

	// Create message
	msg := tgbotapi.MediaGroupConfig{
		ChatID: conf.TelegramChatID,
		Media:  medias,
	}
	
	// Log what we're about to send
	log.Debug.Printf("Sending media group with %d items", len(medias))
	
	messages, err := bot.SendMediaGroup(msg)
	if err != nil {
		log.Error.Printf("Failed to send media group: %s", err.Error())
		// Check if we can determine more about the error
		if strings.Contains(err.Error(), "file must be non-empty") {
			// Try to get more information about the files we're trying to send
			for i, media := range medias {
				switch m := media.(type) {
				case tgbotapi.InputMediaPhoto:
					if filePath, ok := m.Media.(tgbotapi.FilePath); ok {
						fileInfo, statErr := os.Stat(string(filePath))
						if statErr != nil {
							log.Error.Printf("Media item %d: Cannot get file info: %s", i, statErr.Error())
						} else {
							log.Error.Printf("Media item %d: Photo file exists, size: %d bytes", i, fileInfo.Size())
						}
					}
				case tgbotapi.InputMediaVideo:
					if filePath, ok := m.Media.(tgbotapi.FilePath); ok {
						fileInfo, statErr := os.Stat(string(filePath))
						if statErr != nil {
							log.Error.Printf("Media item %d: Cannot get file info: %s", i, statErr.Error())
						} else {
							log.Error.Printf("Media item %d: Video file exists, size: %d bytes", i, fileInfo.Size())
						}
					}
				}
			}
		}
		ErrorSend("Error send media group message: "+err.Error(), bot, FrigateEvent.ID)
	}

	if messages == nil {
		ErrorSend("No received messages", bot, FrigateEvent.ID)
	}

	// Now we can safely remove the files after the media group is sent
	if hasClip {
		os.Remove(FilePathClip)
	}
	os.Remove(FilePathThumbnail)

	var State string
	State = "InProgress"
	if FrigateEvent.EndTime != 0 {
		State = "Finished"
	}
	redis.AddNewEvent(FrigateEvent.ID, State, time.Duration(conf.RedisTTL)*time.Second)
}

func StringsContains(MyStr string, MySlice []string) bool {
	for _, v := range MySlice {
		if v == MyStr {
			return true
		}
	}
	return false
}

func ParseEvents(FrigateEvents EventsStruct, bot *tgbotapi.BotAPI, WatchDog bool) {
	// Parse events
	conf := config.New()
	RedisKeyPrefix := ""
	if WatchDog {
		RedisKeyPrefix = "WatchDog_"
	}
	for Event := range FrigateEvents {
		if !(len(conf.FrigateExcludeCamera) == 1 && conf.FrigateExcludeCamera[0] == "None") {
			if StringsContains(FrigateEvents[Event].Camera, conf.FrigateExcludeCamera) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if !(len(conf.FrigateIncludeCamera) == 1 && conf.FrigateIncludeCamera[0] == "All") {
			if !(StringsContains(FrigateEvents[Event].Camera, conf.FrigateIncludeCamera)) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if !(len(conf.FrigateExcludeLabel) == 1 && conf.FrigateExcludeLabel[0] == "None") {
			if StringsContains(FrigateEvents[Event].Label, conf.FrigateExcludeLabel) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Label)
				continue
			}
		}
		if !(len(conf.FrigateIncludeLabel) == 1 && conf.FrigateIncludeLabel[0] == "All") {
			if !(StringsContains(FrigateEvents[Event].Label, conf.FrigateIncludeLabel)) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Label)
				continue
			}
		}

		if redis.CheckEvent(RedisKeyPrefix + FrigateEvents[Event].ID) {
			if WatchDog {
				SendTextEvent(FrigateEvents[Event], bot)
			} else {
				go SendMessageEvent(FrigateEvents[Event], bot)
			}
		}
	}
}

func SendTextEvent(FrigateEvent EventStruct, bot *tgbotapi.BotAPI) {
	conf := config.New()
	text := "*New event*\n"
	text += "┣*Camera*\n┗ `" + FrigateEvent.Camera + "`\n"
	text += "┣*Label*\n┗ `" + FrigateEvent.Label + "`\n"
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s", t_start) + "`\n"
	text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
	text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
	text += "┣*Zones*\n┗ `" + strings.Join(GetTagList(FrigateEvent.Zones), ", ") + "`\n"
	text += "┣*Event URL*\n┗ " + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",")
	msg := tgbotapi.NewMessage(conf.TelegramChatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := bot.Send(msg)
	if err != nil {
		log.Error.Println(err.Error())
	}
	redis.AddNewEvent("WatchDog_"+FrigateEvent.ID, "Finished", time.Duration(conf.RedisTTL)*time.Second)
}

func NotifyEvents(bot *tgbotapi.BotAPI, FrigateEventsURL string) {
	conf := config.New()
	for {
		FrigateEvents := GetEvents(FrigateEventsURL, bot, false)
		ParseEvents(FrigateEvents, bot, true)
		time.Sleep(time.Duration(conf.WatchDogSleepTime) * time.Second)
	}
}
