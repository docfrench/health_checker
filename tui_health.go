package main

import (
	"context"
	"fmt"
	"net/http"
	"io"
	"os"
	"strings"
    "os/exec"
    "encoding/json"
	"time"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/docker/docker/client"
)

const (
	apiURL      = "https://deimosarchive.com/health"
	interval    = 7200 * time.Second
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
    listHeight = 14
)

type containerTarget struct {
    Label string `json:"label"`
    Name  string `json:"name"`
}


var defaultContainers = []containerTarget{
	{"NPM", "NginxProxyManager"},
	{"Cloudflare-DDNS", "Cloudflare-DDNS"},
}

func loadContainers() []containerTarget {
    data, err := os.ReadFile("/data/config.json")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error reading config.json, using defaults: %v\n", err)
        return defaultContainers
    }

    var targets []containerTarget
    if err := json.Unmarshal(data, &targets); err != nil {
        fmt.Fprintf(os.Stderr, "Error parsing config.json, using defaults: %v\n", err)
        return defaultContainers
    }
    return targets
}

type styles struct {
	title        lipgloss.Style
	item         lipgloss.Style
	selectedItem lipgloss.Style
	pagination   lipgloss.Style
	help         lipgloss.Style
	quitText     lipgloss.Style
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

type model struct {
	list     list.Model
	choice   string
	styles   styles
	quitting bool
    logOutput string
}

func initialModel() model {
	items := []list.Item{
		item("Show tail -n 30"),
        item("Second option"),

	}

	const defaultWidth = 20

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Deimos Archive <> Container Health Checker"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	m := model{list: l}
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
			return m, runTailCmd()
        case "esc":
            m.choice = ""            
            //tea.NewProgram(initialModel(), tea.WithAltScreen())           
            return m, nil
		}
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
        header := m.styles.title.Render("Container Status Log <> Press Esc to return")
        v = tea.NewView(header + "\n\n" + m.logOutput)
	} else if m.quitting {
		v = tea.NewView(m.styles.quitText.Render("Quit."))
	} else {
        v = tea.NewView("\n" + m.list.View())
    }
	v.AltScreen = true
    return v 
}

func main() {
    var targets = loadContainers()

    localTime := time.Now()
    fmt.Println("Local time:", localTime)

    // See what time zone is being used
    fmt.Println("Time zone:", localTime.Location())

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



	// Goroutine: HTTP health check
	go func() {

		for {
			var result string
			//var color string

			resp, err := httpClient.Get(apiURL)
			if err != nil {
				result = fmt.Sprintf("Error making request: %v\n", err)
				//color = colorRed
			} else {
				now := time.Now()
				result = fmt.Sprintf("[Deimos Archive FastAPI] Status: %d <> Time: %s\n", resp.StatusCode, now.Format("2 Jan 06 03:04PM"))
				//color = colorGreen

				if err := resp.Body.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
				}
			}

			logResult(file, result)

			time.Sleep(interval)
		}
	}()

	// One goroutine per container target, all sharing the single Docker client.
	// cli is safe to share across goroutines for reads like ContainerInspect.
	for _, target := range targets {
		target := target // capture loop variable per-iteration
		go func() {

			for {
				checkContainer(cli, file, target)
				time.Sleep(interval)
			}
		}()
	}



	
    if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

func checkContainer(cli *client.Client, file *os.File, target containerTarget) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	inspect, err := cli.ContainerInspect(ctx, target.Name)
	cancel()

	now := time.Now()
	var result string


	if err != nil {
		result = fmt.Sprintf("[%s] Error inspecting container: %v <> Time: %s\n", target.Label, err, now.Format("2 Jan 06 03:04PM"))

	} else {
		status := inspect.State.Status // "running", "exited", etc.
		health := "n/a"
		if inspect.State.Health != nil {
			health = inspect.State.Health.Status // "healthy", "unhealthy", "starting"
		}
		result = fmt.Sprintf("[%s] Status: %s <> Health: %s <> Time: %s\n", target.Label, status, health, now.Format("2 Jan 06 03:04PM"))
	}


	logResult(file, result)
}

func logResult(file *os.File, result string) {
	if _, err := file.WriteString(result); err != nil {
		fmt.Println("Error writing to file:", err)
	}
}
