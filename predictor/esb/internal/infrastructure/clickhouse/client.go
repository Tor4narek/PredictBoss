package clickhouse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

var databasePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type Client struct {
	baseURL  string
	database string
	user     string
	password string
	http     *http.Client
}

func NewClient(baseURL, database, user, password string) (*Client, error) {
	database = strings.TrimSpace(database)
	if !databasePattern.MatchString(database) {
		return nil, errors.New("invalid ClickHouse database name")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), database: database, user: user, password: password, http: &http.Client{Timeout: 45 * time.Second}}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ping", nil)
	if err != nil {
		return err
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ClickHouse returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) Query(ctx context.Context, sql string) ([]byte, error) {
	values := url.Values{}
	values.Set("query", sql)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("ClickHouse response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ClickHouse HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) Insert(ctx context.Context, table string, rows any) error {
	allowed := map[string]bool{
		"news": true, "facts": true, "fact_company": true, "metric_observations": true,
		"metric_totals": true, "company_analyses": true,
	}
	if !allowed[table] {
		return fmt.Errorf("unknown ClickHouse table %q", table)
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	value, err := marshalRows(rows)
	if err != nil {
		return err
	}
	for _, row := range value {
		if err := encoder.Encode(row); err != nil {
			return err
		}
	}
	if body.Len() == 0 {
		return nil
	}
	values := url.Values{}
	values.Set("query", fmt.Sprintf("INSERT INTO `%s`.`%s` FORMAT JSONEachRow", c.database, table))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/?"+values.Encode(), &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("ClickHouse HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func marshalRows(rows any) ([]any, error) {
	encoded, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	var result []any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) authorize(request *http.Request) {
	if c.user != "" {
		request.SetBasicAuth(c.user, c.password)
	}
}

func (c *Client) queryRows(ctx context.Context, sql string, target any) error {
	body, err := c.Query(ctx, sql)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes)
	items := make([]json.RawMessage, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) != 0 {
			items = append(items, append(json.RawMessage(nil), line...))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return json.Unmarshal(mustMarshal(items), target)
}

func mustMarshal(value any) []byte { data, _ := json.Marshal(value); return data }

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return `'` + value + `'`
}

func checkedDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
