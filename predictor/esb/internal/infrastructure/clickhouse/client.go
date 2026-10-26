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

var databasePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type ClientConfig struct {
	RequestTimeout   time.Duration
	MaxResponseBytes int64
}

type Client struct {
	baseURL          string
	database         string
	user             string
	password         string
	http             *http.Client
	maxResponseBytes int64
}

func NewClient(baseURL, database, user, password string, config ClientConfig) (*Client, error) {
	database = strings.TrimSpace(database)
	if !databasePattern.MatchString(database) {
		return nil, errors.New("invalid ClickHouse database name")
	}
	if config.RequestTimeout <= 0 || config.MaxResponseBytes <= 0 {
		return nil, errors.New("ClickHouse timeout and response limit must be positive")
	}
	parsedURL, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || !parsedURL.IsAbs() || parsedURL.Host == "" ||
		(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, errors.New("invalid ClickHouse HTTP URL")
	}
	return &Client{baseURL: strings.TrimRight(parsedURL.String(), "/"), database: database, user: user, password: password,
		http: &http.Client{
			Timeout: config.RequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}, maxResponseBytes: config.MaxResponseBytes}, nil
}

func (c *Client) Ping(ctx context.Context) (resultErr error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ping", nil)
	if err != nil {
		return err
	}
	c.authorize(request)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeResponseBody(response.Body)) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ClickHouse returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) Query(ctx context.Context, sql string) (body []byte, resultErr error) {
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
	defer func() { resultErr = errors.Join(resultErr, closeResponseBody(response.Body)) }()
	body, err = io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.maxResponseBytes {
		return nil, errors.New("ClickHouse response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ClickHouse returned HTTP %d", response.StatusCode)
	}
	return body, nil
}

func (c *Client) Insert(ctx context.Context, table string, rows any) (resultErr error) {
	allowed := map[string]bool{
		"news": true, "news_content": true, "news_processing": true,
		"facts": true, "fact_company": true, "metric_observations": true,
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
	defer func() { resultErr = errors.Join(resultErr, closeResponseBody(response.Body)) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ClickHouse returned HTTP %d", response.StatusCode)
	}
	return nil
}

func closeResponseBody(body io.Closer) error {
	if err := body.Close(); err != nil {
		return fmt.Errorf("close ClickHouse response body: %w", err)
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
	scanner.Buffer(make([]byte, 64*1024), int(c.maxResponseBytes))
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
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode ClickHouse rows: %w", err)
	}
	return json.Unmarshal(encoded, target)
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return `'` + value + `'`
}
