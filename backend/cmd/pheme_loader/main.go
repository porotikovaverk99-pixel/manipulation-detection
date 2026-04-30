package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/audittrail"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

const twitterTimeLayout = "Mon Jan 02 15:04:05 -0700 2006"

type phemePathMeta struct {
	ArchiveType    string
	EventName      string
	Label          string
	CaseExternalID string
	EntryType      string
	PostExternalID string
}

type phemeArchiveCase struct {
	ArchiveType    string
	EventName      string
	Label          string
	CaseExternalID string
	Annotation     map[string]interface{}
	Structure      json.RawMessage
	SourceTweet    *phemeTweetEnvelope
	Reactions      []*phemeTweetEnvelope
}

type phemeTweetEnvelope struct {
	Tweet  phemeTweet
	Raw    []byte
	IsRoot bool
}

type phemeTweet struct {
	IDStr                string `json:"id_str"`
	CreatedAt            string `json:"created_at"`
	Text                 string `json:"text"`
	Lang                 string `json:"lang"`
	InReplyToStatusIDStr string `json:"in_reply_to_status_id_str"`
	RetweetCount         int    `json:"retweet_count"`
	FavoriteCount        int    `json:"favorite_count"`
	User                 struct {
		IDStr          string `json:"id_str"`
		ScreenName     string `json:"screen_name"`
		Name           string `json:"name"`
		FollowersCount int    `json:"followers_count"`
		FriendsCount   int    `json:"friends_count"`
		Verified       bool   `json:"verified"`
	} `json:"user"`
	Entities struct {
		Hashtags []struct {
			Text string `json:"text"`
		} `json:"hashtags"`
		URLs []struct {
			URL         string `json:"url"`
			ExpandedURL string `json:"expanded_url"`
			DisplayURL  string `json:"display_url"`
		} `json:"urls"`
	} `json:"entities"`
}

func main() {
	_ = godotenv.Load()
	logFile, err := applog.ConfigureStandardLog("pheme_loader")
	if err != nil {
		log.Printf("configure file logging failed: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	archivePath := strings.TrimSpace(os.Getenv("PHEME_ARCHIVE_PATH"))
	if archivePath == "" {
		log.Fatal("PHEME_ARCHIVE_PATH is required")
	}

	datasetName := getenvDefault("DATASET_NAME", "pheme")
	datasetSplit := getenvDefault("DATASET_SPLIT", "all")
	sourceName := getenvDefault("SOURCE_NAME", "pheme")
	labelFilter := parseFilterSet(os.Getenv("PHEME_LABEL_FILTER"))
	if len(labelFilter) > 0 {
		normalized := make(map[string]struct{}, len(labelFilter))
		for label := range labelFilter {
			normalized[normalizePhemeLabel(label)] = struct{}{}
		}
		labelFilter = normalized
	}
	eventFilter := parseFilterSet(os.Getenv("PHEME_EVENT_FILTER"))
	caseLimit := getenvInt("PHEME_LIMIT_CASES", 0)

	connStr := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if connStr == "" {
		connStr = strings.TrimSpace(os.Getenv("DATABASE_URI"))
	}
	if connStr == "" {
		log.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	job := audittrail.NewCLIJob(db, "pheme_loader").
		WithSource("dataset", sourceName, datasetName, datasetSplit).
		WithPayload(map[string]interface{}{
			"archive_path": archivePath,
			"case_limit":   caseLimit,
		})
	job.Start()
	defer job.FinishAndExit()

	sourceID, err := db.EnsureDataSource(sourceName, "")
	if err != nil {
		job.Failf("ensure data source failed: %v", err)
		return
	}

	runID, err := db.StartIngestionRun("dataset", sourceName, datasetName, datasetSplit)
	if err != nil {
		job.Failf("start ingestion run failed: %v", err)
		return
	}
	job.Set("ingestion_run_id", runID)

	runStatus := "completed"
	runNotes := "ok"
	persistedCases := 0
	persistedPosts := 0

	defer func() {
		if runNotes == "ok" {
			runNotes = fmt.Sprintf("cases=%d posts=%d", persistedCases, persistedPosts)
		}
		if err := db.FinishIngestionRun(runID, runStatus, runNotes); err != nil {
			log.Printf("finish ingestion run failed: %v", err)
		}
	}()

	cases, err := readPhemeArchive(archivePath, eventFilter, labelFilter, caseLimit)
	if err != nil {
		runStatus = "failed"
		runNotes = "read archive failed: " + err.Error()
		job.Failf("%s", runNotes)
		return
	}

	for _, c := range cases {
		postCount, err := persistPhemeCase(db, sourceID, sourceName, datasetName, datasetSplit, runID, c)
		if err != nil {
			runStatus = "failed"
			runNotes = fmt.Sprintf("persist case %s failed: %v", c.CaseExternalID, err)
			job.Failf("%s", runNotes)
			return
		}
		persistedCases++
		persistedPosts += postCount
	}

	job.Set("persisted_cases", persistedCases)
	job.Set("persisted_posts", persistedPosts)
	job.Set("run_status", runStatus)
	log.Printf("pheme loader completed: cases=%d posts=%d run_id=%d", persistedCases, persistedPosts, runID)
}

func readPhemeArchive(archivePath string, eventFilter, labelFilter map[string]struct{}, caseLimit int) ([]*phemeArchiveCase, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f
	switch {
	case strings.HasSuffix(archivePath, ".tar.bz2"):
		reader = bzip2.NewReader(f)
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer gzr.Close()
		reader = gzr
	default:
		return nil, fmt.Errorf("unsupported archive type: %s", archivePath)
	}

	tr := tar.NewReader(reader)
	var out []*phemeArchiveCase
	var current *phemeArchiveCase
	currentKey := ""

	flushCurrent := func() {
		if current == nil {
			return
		}
		if current.SourceTweet != nil {
			out = append(out, current)
		}
		current = nil
		currentKey = ""
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		meta, ok := parsePhemePath(hdr.Name)
		if !ok {
			continue
		}
		if len(eventFilter) > 0 {
			if _, allowed := eventFilter[meta.EventName]; !allowed {
				continue
			}
		}
		if len(labelFilter) > 0 {
			if _, allowed := labelFilter[meta.Label]; !allowed {
				continue
			}
		}

		key := meta.ArchiveType + "|" + meta.EventName + "|" + meta.Label + "|" + meta.CaseExternalID
		if currentKey != "" && key != currentKey {
			flushCurrent()
			if caseLimit > 0 && len(out) >= caseLimit {
				break
			}
		}
		if current == nil {
			current = &phemeArchiveCase{
				ArchiveType:    meta.ArchiveType,
				EventName:      meta.EventName,
				Label:          meta.Label,
				CaseExternalID: meta.CaseExternalID,
			}
			currentKey = key
		}

		payload, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read entry %s: %w", hdr.Name, err)
		}

		switch meta.EntryType {
		case "annotation":
			if len(payload) > 0 {
				var annotation map[string]interface{}
				if err := json.Unmarshal(payload, &annotation); err != nil {
					return nil, fmt.Errorf("parse annotation %s: %w", hdr.Name, err)
				}
				current.Annotation = annotation
			}
		case "structure":
			if len(payload) > 0 {
				current.Structure = append([]byte(nil), payload...)
			}
		case "source", "reaction":
			var tweet phemeTweet
			if err := json.Unmarshal(payload, &tweet); err != nil {
				return nil, fmt.Errorf("parse tweet %s: %w", hdr.Name, err)
			}
			env := &phemeTweetEnvelope{
				Tweet:  tweet,
				Raw:    append([]byte(nil), payload...),
				IsRoot: meta.EntryType == "source",
			}
			if env.IsRoot {
				current.SourceTweet = env
			} else {
				current.Reactions = append(current.Reactions, env)
			}
		}
	}

	flushCurrent()
	if caseLimit > 0 && len(out) > caseLimit {
		out = out[:caseLimit]
	}

	return out, nil
}

func persistPhemeCase(
	db *repository.PostgresDB,
	sourceID int,
	sourceName, datasetName, datasetSplit string,
	runID int64,
	c *phemeArchiveCase,
) (int, error) {
	if c.SourceTweet == nil {
		return 0, fmt.Errorf("case %s has no source tweet", c.CaseExternalID)
	}

	allTweets := make([]*phemeTweetEnvelope, 0, 1+len(c.Reactions))
	allTweets = append(allTweets, c.SourceTweet)
	allTweets = append(allTweets, c.Reactions...)

	firstEventAt, lastEventAt := caseTimeBounds(allTweets)

	caseMetadata := map[string]interface{}{
		"event_name":     c.EventName,
		"archive_type":   c.ArchiveType,
		"reaction_count": len(c.Reactions),
		"has_annotation": c.Annotation != nil,
		"has_structure":  len(c.Structure) > 0,
		"source_dataset": "pheme",
		"label_original": c.Label,
	}
	if len(c.Annotation) > 0 {
		caseMetadata["annotation"] = c.Annotation
	}
	if len(c.Structure) > 0 {
		var structure interface{}
		if err := json.Unmarshal(c.Structure, &structure); err == nil {
			caseMetadata["structure"] = structure
		}
	}

	caseID, err := db.UpsertCase(repository.CaseRecord{
		SourceType:         "dataset",
		SourceName:         sourceName,
		DatasetName:        datasetName,
		DatasetSplit:       datasetSplit,
		ExternalCaseID:     c.CaseExternalID,
		CaseType:           "thread",
		Label:              c.Label,
		Title:              trimForTitle(c.SourceTweet.Tweet.Text),
		EventName:          c.EventName,
		RootPostExternalID: c.SourceTweet.Tweet.IDStr,
		Status:             "closed",
		OpenedAt:           &firstEventAt,
		ClosedAt:           &lastEventAt,
		FirstEventAt:       &firstEventAt,
		LastEventAt:        &lastEventAt,
		Metadata:           caseMetadata,
	})
	if err != nil {
		return 0, err
	}

	sort.SliceStable(c.Reactions, func(i, j int) bool {
		return parseTwitterTimestamp(c.Reactions[i].Tweet.CreatedAt).Before(parseTwitterTimestamp(c.Reactions[j].Tweet.CreatedAt))
	})

	postCount := 0
	rootPost := mapTweetToDatasetPost(sourceID, datasetName, datasetSplit, c, caseID, runID, c.SourceTweet)
	if err := db.SaveDatasetPost(rootPost); err != nil {
		return postCount, fmt.Errorf("save source tweet: %w", err)
	}
	postCount++

	for _, reaction := range c.Reactions {
		post := mapTweetToDatasetPost(sourceID, datasetName, datasetSplit, c, caseID, runID, reaction)
		if err := db.SaveDatasetPost(post); err != nil {
			return postCount, fmt.Errorf("save reaction %s: %w", reaction.Tweet.IDStr, err)
		}
		postCount++
	}

	return postCount, nil
}

func mapTweetToDatasetPost(
	sourceID int,
	datasetName, datasetSplit string,
	c *phemeArchiveCase,
	caseID int64,
	runID int64,
	env *phemeTweetEnvelope,
) repository.DatasetPost {
	accountExternalID := strings.TrimSpace(env.Tweet.User.IDStr)
	if accountExternalID == "" {
		accountExternalID = "pheme-author:" + env.Tweet.IDStr
	}

	username := strings.TrimSpace(env.Tweet.User.ScreenName)
	if username == "" {
		username = "pheme_user_" + accountExternalID
	}

	displayName := strings.TrimSpace(env.Tweet.User.Name)
	if displayName == "" {
		displayName = username
	}

	postMetadata := map[string]interface{}{
		"dataset":              "pheme",
		"event_name":           c.EventName,
		"case_label":           c.Label,
		"archive_type":         c.ArchiveType,
		"is_source_tweet":      env.IsRoot,
		"in_reply_to_external": strings.TrimSpace(env.Tweet.InReplyToStatusIDStr),
	}

	postCaseID := caseID
	return repository.DatasetPost{
		SourceID:          sourceID,
		ExternalID:        env.Tweet.IDStr,
		AccountExternalID: accountExternalID,
		Username:          username,
		DisplayName:       displayName,
		Content:           env.Tweet.Text,
		Language:          env.Tweet.Lang,
		PublishedAt:       parseTwitterTimestamp(env.Tweet.CreatedAt),
		LikesCount:        env.Tweet.FavoriteCount,
		RepostsCount:      env.Tweet.RetweetCount,
		RawPayloadRef:     fmt.Sprintf("pheme:%s:%s:%s:%s", c.EventName, c.Label, c.CaseExternalID, env.Tweet.IDStr),
		RawPayloadHash:    sha256Hex(env.Raw),
		DatasetName:       datasetName,
		DatasetSplit:      datasetSplit,
		DatasetRecordID:   env.Tweet.IDStr,
		IngestionRunID:    &runID,
		CaseID:            &postCaseID,
		IsCaseRoot:        env.IsRoot,
		ReplyToExternalID: strings.TrimSpace(env.Tweet.InReplyToStatusIDStr),
		FollowersCount:    env.Tweet.User.FollowersCount,
		FollowingCount:    env.Tweet.User.FriendsCount,
		IsVerified:        env.Tweet.User.Verified,
		Metadata:          postMetadata,
		Tags:              collectHashtags(env.Tweet),
		Links:             collectLinks(env.Tweet),
	}
}

func parsePhemePath(name string) (phemePathMeta, bool) {
	clean := path.Clean(name)
	if clean == "." || strings.Contains(clean, "._") || strings.Contains(clean, ".DS_Store") {
		return phemePathMeta{}, false
	}
	parts := strings.Split(clean, "/")
	if len(parts) < 5 {
		return phemePathMeta{}, false
	}

	switch parts[0] {
	case "pheme-rnr-dataset":
		if len(parts) < 6 {
			return phemePathMeta{}, false
		}
		meta := phemePathMeta{
			ArchiveType:    "rnr",
			EventName:      parts[1],
			Label:          normalizePhemeLabel(parts[2]),
			CaseExternalID: parts[3],
		}
		switch parts[4] {
		case "source-tweet":
			meta.EntryType = "source"
			meta.PostExternalID = strings.TrimSuffix(parts[5], ".json")
			return meta, true
		case "reactions":
			meta.EntryType = "reaction"
			meta.PostExternalID = strings.TrimSuffix(parts[5], ".json")
			return meta, true
		default:
			return phemePathMeta{}, false
		}
	case "all-rnr-annotated-threads":
		if len(parts) < 5 {
			return phemePathMeta{}, false
		}
		meta := phemePathMeta{
			ArchiveType:    "veracity",
			EventName:      strings.TrimSuffix(parts[1], "-all-rnr-threads"),
			Label:          normalizePhemeLabel(parts[2]),
			CaseExternalID: parts[3],
		}
		switch parts[4] {
		case "annotation.json":
			meta.EntryType = "annotation"
			return meta, true
		case "structure.json":
			meta.EntryType = "structure"
			return meta, true
		case "source-tweets":
			if len(parts) < 6 {
				return phemePathMeta{}, false
			}
			meta.EntryType = "source"
			meta.PostExternalID = strings.TrimSuffix(parts[5], ".json")
			return meta, true
		case "reactions":
			if len(parts) < 6 {
				return phemePathMeta{}, false
			}
			meta.EntryType = "reaction"
			meta.PostExternalID = strings.TrimSuffix(parts[5], ".json")
			return meta, true
		default:
			return phemePathMeta{}, false
		}
	default:
		return phemePathMeta{}, false
	}
}

func normalizePhemeLabel(label string) string {
	switch strings.TrimSpace(strings.ToLower(label)) {
	case "rumours", "rumour":
		return "rumour"
	case "non-rumours", "nonrumour", "non-rumour", "non_rumour":
		return "non_rumour"
	default:
		return strings.TrimSpace(strings.ToLower(label))
	}
}

func parseTwitterTimestamp(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(twitterTimeLayout, raw)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func caseTimeBounds(tweets []*phemeTweetEnvelope) (time.Time, time.Time) {
	if len(tweets) == 0 {
		now := time.Now().UTC()
		return now, now
	}
	first := parseTwitterTimestamp(tweets[0].Tweet.CreatedAt)
	last := first
	for _, tweet := range tweets[1:] {
		ts := parseTwitterTimestamp(tweet.Tweet.CreatedAt)
		if ts.Before(first) {
			first = ts
		}
		if ts.After(last) {
			last = ts
		}
	}
	return first, last
}

func collectHashtags(tweet phemeTweet) []string {
	out := make([]string, 0, len(tweet.Entities.Hashtags))
	for _, hashtag := range tweet.Entities.Hashtags {
		if strings.TrimSpace(hashtag.Text) == "" {
			continue
		}
		out = append(out, hashtag.Text)
	}
	return out
}

func collectLinks(tweet phemeTweet) []repository.DatasetLink {
	out := make([]repository.DatasetLink, 0, len(tweet.Entities.URLs))
	for _, item := range tweet.Entities.URLs {
		rawURL := strings.TrimSpace(item.URL)
		expanded := strings.TrimSpace(item.ExpandedURL)
		if rawURL == "" && expanded == "" {
			continue
		}
		linkDomain := linkDomain(firstNonEmpty(expanded, rawURL))
		out = append(out, repository.DatasetLink{
			URL:         firstNonEmpty(rawURL, expanded),
			ExpandedURL: expanded,
			Domain:      linkDomain,
		})
	}
	return out
}

func linkDomain(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func trimForTitle(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 240 {
		return text
	}
	return text[:240]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseFilterSet(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
