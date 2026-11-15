package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Args is the process context handed to Run.
type Args struct {
	Argv      []string
	Version   string
	Commit    string
	Date      string
	Stdout    io.Writer
	Stderr    io.Writer
	IsTTY     bool
	Platform  string
	GoVersion string
}

// options is the parsed command line.
type options struct {
	cmd      string
	rangeArg string
	since    string
	until    string
	json     bool
	byAgent  bool
	rebuild  bool
	noSync   bool
	noMagpie bool
	noColor  bool
	usd      bool
	width    int
	top      int
	initCfg  bool
	help     bool
}

// parse turns argv into options. Flags may appear before or after the range
// argument, and every flag has a long and a one-letter form.
func parse(argv []string) (options, error) {
	o := options{}
	needsValue := map[string]bool{
		"since": true, "until": true, "width": true, "top": true,
		"s": true, "u": true, "w": true, "n": true,
	}
	i := 0
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name := strings.TrimLeft(arg, "-")
			val := ""
			hasVal := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				val, name, hasVal = name[eq+1:], name[:eq], true
			}
			if needsValue[name] && !hasVal {
				if i+1 >= len(argv) {
					return o, fmt.Errorf("flag --%s needs a value", name)
				}
				val = argv[i+1]
				i++
			}
			switch name {
			case "since", "s":
				o.since = val
			case "until", "u":
				o.until = val
			case "width", "w":
				n, err := strconv.Atoi(val)
				if err != nil {
					return o, fmt.Errorf("--width needs a number")
				}
				o.width = n
			case "top", "n":
				n, err := strconv.Atoi(val)
				if err != nil {
					return o, fmt.Errorf("--top needs a number")
				}
				o.top = n
			case "json", "j":
				o.json = true
			case "by-agent", "a":
				o.byAgent = true
			case "rebuild":
				o.rebuild = true
			case "no-sync":
				o.noSync = true
			case "no-magpie":
				o.noMagpie = true
			case "no-color", "no-ansi", "no-colour":
				o.noColor = true
			case "usd":
				o.usd = true
			case "init":
				o.initCfg = true
			case "help", "h", "?":
				o.help = true
			case "version", "v":
				o.cmd = "version"
			default:
				return o, fmt.Errorf("unknown flag --%s", name)
			}
			i++
			continue
		}
		lower := strings.ToLower(arg)
		switch lower {
		case "doctor", "adapters", "config", "version", "help", "completion":
			if o.cmd == "" {
				o.cmd = lower
			} else if o.rangeArg == "" {
				o.rangeArg = arg
			}
		default:
			if o.rangeArg == "" {
				o.rangeArg = arg
			} else if o.cmd == "" {
				o.cmd = lower
			}
		}
		i++
	}
	return o, nil
}
