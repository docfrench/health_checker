package main

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
    "sort"
)

const (
	apiURL      = "https://deimosarchive.com/health"
	interval    = 120 * time.Second
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	listHeight  = 14
)

type containerTarget struct {
	Label string `json:"label"`
	Name  string `json:"name"`
}

type httpTarget struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type appConfig struct {
    ServerName    string            `json:"server_name"`
	Containers    []containerTarget `json:"containers"`
	HTTPEndpoints []httpTarget      `json:"http_endpoints"`
}

var defaultConfig = appConfig{
    ServerName: "My Server",
	Containers: []containerTarget{
		{"NPM", "NginxProxyManager"},
		{"Cloudflare-DDNS", "cloudflare-ddns"},
	},
	HTTPEndpoints: []httpTarget{
		{"Deimos Archive FastAPI", "https://deimosarchive.com/health"},
	},
}

type model struct {
	list         list.Model
	choice       string
	styles       styles
	quitting     bool
	logOutput    string
	statusOutput string
    serverName   string
}

type tickMsg time.Time
type statusMsg string

type styles struct {
	title        lipgloss.Style
	item         lipgloss.Style
	selectedItem lipgloss.Style
	pagination   lipgloss.Style
	help         lipgloss.Style
	quitText     lipgloss.Style
}

type ContainerStatus struct {
	Label     string    `json:"label"`
	Status    string    `json:"status"`
	Health    string    `json:"health"`
	CheckedAt time.Time `json:"checked_at"`
}

type HTTPStatus struct {
	Label      string    `json:"label"`
	StatusCode int       `json:"status_code"`
	CheckedAt  time.Time `json:"checked_at"`
}

type Snapshot struct {
	UpdatedAt  time.Time         `json:"updated_at"`
	Containers []ContainerStatus `json:"containers"`
	HTTPChecks []HTTPStatus      `json:"http_checks"`
}

type statusStore struct {
	mu         sync.Mutex
	containers map[string]ContainerStatus
	http       map[string]HTTPStatus
}

func newStatusStore() *statusStore {
	return &statusStore{
		containers: make(map[string]ContainerStatus),
		http:       make(map[string]HTTPStatus),
	}
}

func (s *statusStore) setContainer(cs ContainerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers[cs.Label] = cs
}

func (s *statusStore) setHTTP(hs HTTPStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.http[hs.Label] = hs
}

func (s *statusStore) snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{UpdatedAt: time.Now()}
	for _, c := range s.containers {
		snap.Containers = append(snap.Containers, c)
	}
	for _, h := range s.http {
		snap.HTTPChecks = append(snap.HTTPChecks, h)
	}
	return snap
}

func loadConfig() appConfig {
	data, err := os.ReadFile("/data/config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config.json, using defaults: %v\n", err)
		return defaultConfig
	}

	var cfg appConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing config.json, using defaults: %v\n", err)
		return defaultConfig
	}
	return cfg
}

func newStyles(darkBG bool) styles {
	var s styles
	s.title = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("178"))
	s.item = lipgloss.NewStyle().PaddingLeft(4)
	s.selectedItem = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	s.pagination = list.DefaultStyles(darkBG).PaginationStyle.PaddingLeft(4)
	s.help = list.DefaultStyles(darkBG).HelpStyle.PaddingLeft(4).PaddingBottom(1)
	s.quitText = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	return s
}

type item string
type logMsg string

func (i item) FilterValue() string { return "" }

type itemDelegate struct {
	styles *styles
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := d.styles.item.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return d.styles.selectedItem.Render("> " + strings.Join(s, " "))
		}
	}

	_, _ = fmt.Fprint(w, fn(str))
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func readStatus() (string, error) {
	data, err := os.ReadFile("/data/status.json")
	var snap Snapshot
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing status.json: %v\n", err)
		return "", err
	}
	return formatSnapshot(snap), nil
}

func formatSnapshot(snap Snapshot) string {
    var b strings.Builder

    // sort the Containers by Label (A-Z)
    sort.Slice(snap.Containers, func(i, j int) bool {
        return snap.Containers[i].Label < snap.Containers[j].Label
    })
    // sort the HTTP/API by Label (A-Z)
    sort.Slice(snap.HTTPChecks, func(i, j int) bool {
        return snap.HTTPChecks[i].Label < snap.HTTPChecks[j].Label
    })

    fmt.Fprintf(&b, "Updated: %s\n\n", snap.UpdatedAt.Format("2 Jan 06 03:04:05PM"))

    for _, c := range snap.Containers {
        color := colorGreen
        if c.Status != "running" {
            color = colorYellow
        }
        if c.Status == "error" {
            color = colorRed
        }
        fmt.Fprintf(&b, "%s%-25s%s Status: %-10s Health: %-10s Checked: %s\n",
            color, c.Label, colorReset, c.Status, c.Health, c.CheckedAt.Format("03:04:05PM"))
    }

    b.WriteString("\n")

    for _, h := range snap.HTTPChecks {
        color := colorGreen
        if h.StatusCode == 0 || h.StatusCode >= 400 {
            color = colorRed
        } else if h.StatusCode >= 300 {
            color = colorYellow
        }
        fmt.Fprintf(&b, "%s%-25s%s Status: %-29d Checked: %s\n",
            color, h.Label, colorReset, h.StatusCode, h.CheckedAt.Format("03:04:05PM"))
    }

    return b.String()
}


func readStatusCmd() tea.Cmd {
	return func() tea.Msg {
		output, err := readStatus()
		if err != nil {
			return statusMsg(fmt.Sprintf("error reading status: %v", err))
		}
		return statusMsg(output)
	}
}

func initialModel(config appConfig) model {
	items := []list.Item{
		item("Show tail -n 30"),
		item("Live Monitoring"),
	}

	const defaultWidth = 20

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = fmt.Sprintf("%s <> Container Health Checker", config.ServerName)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	m := model{list: l, serverName: config.ServerName}
	m.updateStyles(true) // default to dark styles.
	return m
}

func (m *model) updateStyles(isDark bool) {
	m.styles = newStyles(isDark)
	m.list.Styles.Title = m.styles.title
	m.list.Styles.PaginationStyle = m.styles.pagination
	m.list.Styles.HelpStyle = m.styles.help
	m.list.SetDelegate(itemDelegate{styles: &m.styles})
}

func (m model) Init() tea.Cmd {
	return nil
}

func runTailCmd() tea.Cmd {
	return func() tea.Msg {
		output, err := tailLog()
		if err != nil {
			return logMsg(fmt.Sprintf("error reading log: %v", err))
		}
		return logMsg(output)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyPressMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			selected, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = string(selected)
			}
			if m.choice == "Live Monitoring" {
				return m, tea.Batch(readStatusCmd(), tickCmd())
			}
			return m, runTailCmd()
		case "esc":
			m.choice = ""
			return m, nil
		}
	case tickMsg:
		if m.choice == "Live Monitoring" {
			return m, tea.Batch(readStatusCmd(), tickCmd())
		}
		return m, nil
	case statusMsg:
		m.statusOutput = string(msg)
		return m, nil

	case logMsg:
		m.logOutput = string(msg)
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func tailLog() (string, error) {
	out, err := exec.Command("tail", "-n", "30", "/data/health_checker.syslog").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}



func (m model) View() tea.View {
	var v tea.View
	if m.choice == "Show tail -n 30" {
		header := m.styles.title.Render(fmt.Sprintf("%s <> Container Status Log <> Press Esc to return", m.serverName))
		v = tea.NewView(header + "\n\n" + m.logOutput)
	} else if m.choice == "Live Monitoring" {
		header := m.styles.title.Render(fmt.Sprintf("%s <> Live Container Status <> Press Esc to return", m.serverName))
		v = tea.NewView(header + "\n\n" + m.statusOutput)
	} else if m.quitting {
		v = tea.NewView(m.styles.quitText.Render("quitting now!"))
	} else {
		v = tea.NewView("\n" + m.list.View())
	}
	v.AltScreen = true
	return v
}

func writeStatusFile(store *statusStore) {
	snap := store.snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling status: %v\n", err)
		return
	}

	tmpPath := "/data/status.json.tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing status tmp file: %v\n", err)
		return
	}
	if err := os.Rename(tmpPath, "/data/status.json"); err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming status file: %v\n", err)
	}
}

func checkContainer(cli *client.Client, file *os.File, store *statusStore, target containerTarget) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	inspect, err := cli.ContainerInspect(ctx, target.Name)
	cancel()

	now := time.Now()
	var result string
	cs := ContainerStatus{Label: target.Label, CheckedAt: now}

	if err != nil {
		result = fmt.Sprintf("[%s] Error inspecting container: %v <> Time: %s\n", target.Label, err, now.Format("2 Jan 06 03:04PM"))
		cs.Status = "error"
		cs.Health = "n/a"
	} else {
		cs.Status = inspect.State.Status
		cs.Health = "n/a"
		if inspect.State.Health != nil {
			cs.Health = inspect.State.Health.Status
		}
		result = fmt.Sprintf("[%s] Status: %s <> Health: %s <> Time: %s\n", target.Label, cs.Status, cs.Health, now.Format("2 Jan 06 03:04PM"))
	}

	store.setContainer(cs)
	logResult(file, result)
}

func logResult(file *os.File, result string) {
	if _, err := file.WriteString(result); err != nil {
		fmt.Println("Error writing to file:", err)
	}
}

func startStatusWriter(store *statusStore) {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			writeStatusFile(store)
		}
	}()
}

func main() {
	config := loadConfig()

	file, err := os.OpenFile("/data/health_checker.syslog", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Printf("Error creating Docker client: %v\n", err)
		return
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing Docker client: %v\n", err)
		}
	}()

	store := newStatusStore()
	startStatusWriter(store)

	// HTTP API health check
	for _, target := range config.HTTPEndpoints {
		target := target
		go func() {
			for {
				now := time.Now()
				hs := HTTPStatus{Label: target.Label, CheckedAt: now}
				var result string
				resp, err := httpClient.Get(target.URL)
				if err != nil {
					result = fmt.Sprintf("[%s] Error making request: %v\n", target.Label, err)
					hs.StatusCode = 0
				} else {
					hs.StatusCode = resp.StatusCode
					result = fmt.Sprintf("[%s] Status: %d <> Time: %s\n", target.Label, resp.StatusCode, now.Format("2 Jan 06 03:04PM"))
					if err := resp.Body.Close(); err != nil {
						fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
					}
				}
				store.setHTTP(hs)
				logResult(file, result)
				time.Sleep(interval)
			}
		}()
	}

	// Container health check
	for _, target := range config.Containers {
		target := target // capture loop variable per-iteration
		go func() {

			for {
				checkContainer(cli, file, store, target)
				time.Sleep(interval)
			}
		}()
	}

	if len(os.Args) > 1 && os.Args[1] == "--tui" {
        if _, err := tea.NewProgram(initialModel(config)).Run(); err != nil {
            fmt.Println("Error running program:", err)
            os.Exit(1)
        }
        return
    }
    select {}
}
