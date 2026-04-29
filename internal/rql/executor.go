package rql

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/niksingh2745/rawth/internal/storage"
)

// the output of our hard work. if ok is false, something went wrong 
// and message usually tells us who to blame.
type Result struct {
	Ok      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Value   string      `json:"value,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// the executor is the one that actually does the work. 
// it's the bridge between the parser's intentions and the engine's reality.
type Executor struct {
	engine *storage.Engine
}

func NewExecutor(engine *storage.Engine) *Executor {
	return &Executor{engine: engine}
}

// take a string, parse it, run it. if any of that fails, return an error.
func (e *Executor) Execute(input string) Result {
	input = strings.TrimSpace(input)
	if input == "" {
		return Result{Ok: false, Message: "empty command — try HELP"}
	}

	cmd, err := Parse(input)
	if err != nil {
		return Result{Ok: false, Message: fmt.Sprintf("parse error: %s", err)}
	}

	return e.executeCommand(cmd)
}

// dispatch the command. SHOVE, YOINK, YEET... 
// i spend way too much time thinking about these names.
func (e *Executor) executeCommand(cmd *Command) Result {
	switch cmd.Type {
	case CMD_SHOVE:
		return e.execShove(cmd)
	case CMD_YOINK:
		return e.execYoink(cmd)
	case CMD_YEET:
		return e.execYeet(cmd)
	case CMD_PEEK:
		return e.execPeek(cmd)
	case CMD_KEYS:
		return e.execKeys()
	case CMD_NUKE:
		return e.execNuke()
	case CMD_STATS:
		return e.execStats()
	case CMD_HELP:
		return e.execHelp()
	default:
		return Result{Ok: false, Message: "unknown command type"}
	}
}

func (e *Executor) execShove(cmd *Command) Result {
	err := e.engine.Put([]byte(cmd.Key), []byte(cmd.Value), cmd.TTL)
	if err != nil {
		return Result{Ok: false, Message: fmt.Sprintf("SHOVE failed: %s", err)}
	}

	msg := fmt.Sprintf("OK — shoved %q", cmd.Key)
	if cmd.TTL > 0 {
		msg += fmt.Sprintf(" (expires in %ds)", cmd.TTL)
	}
	return Result{Ok: true, Message: msg}
}

func (e *Executor) execYoink(cmd *Command) Result {
	val, err := e.engine.Get([]byte(cmd.Key))
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return Result{Ok: false, Message: fmt.Sprintf("key %q not found — nothing to yoink", cmd.Key)}
		}
		return Result{Ok: false, Message: fmt.Sprintf("YOINK failed: %s", err)}
	}

	return Result{Ok: true, Value: string(val), Message: fmt.Sprintf("yoinked %q", cmd.Key)}
}

func (e *Executor) execYeet(cmd *Command) Result {
	err := e.engine.Delete([]byte(cmd.Key))
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return Result{Ok: false, Message: fmt.Sprintf("key %q not found — can't yeet what doesn't exist", cmd.Key)}
		}
		return Result{Ok: false, Message: fmt.Sprintf("YEET failed: %s", err)}
	}

	return Result{Ok: true, Message: fmt.Sprintf("yeeted %q into the void", cmd.Key)}
}

func (e *Executor) execPeek(cmd *Command) Result {
	exists, err := e.engine.Has([]byte(cmd.Key))
	if err != nil {
		return Result{Ok: false, Message: fmt.Sprintf("PEEK failed: %s", err)}
	}

	if exists {
		return Result{Ok: true, Message: fmt.Sprintf("key %q exists — it's in there", cmd.Key)}
	}
	return Result{Ok: true, Message: fmt.Sprintf("key %q does not exist — the void stares back", cmd.Key)}
}

func (e *Executor) execKeys() Result {
	keys, err := e.engine.Keys()
	if err != nil {
		return Result{Ok: false, Message: fmt.Sprintf("KEYS failed: %s", err)}
	}

	if len(keys) == 0 {
		return Result{Ok: true, Message: "no keys — the database is empty, like my soul", Data: []string{}}
	}

	keyStrings := make([]string, len(keys))
	for i, k := range keys {
		keyStrings[i] = string(k)
	}

	return Result{
		Ok:      true,
		Message: fmt.Sprintf("%d key(s) found", len(keys)),
		Data:    keyStrings,
	}
}

func (e *Executor) execNuke() Result {
	err := e.engine.Nuke()
	if err != nil {
		return Result{Ok: false, Message: fmt.Sprintf("NUKE failed: %s", err)}
	}

	return Result{Ok: true, Message: "💥 NUKED — everything is gone. hope you meant that."}
}

func (e *Executor) execStats() Result {
	stats := e.engine.Stats()
	return Result{
		Ok:      true,
		Message: "database statistics",
		Data:    stats,
	}
}

func (e *Executor) execHelp() Result {
	help := `rawth Query Language (RQL) — Reference

  SHOVE key "value"          Store a key-value pair
  SHOVE key "value" TTL 60   Store with expiration (seconds)
  YOINK key                  Retrieve a value by key
  YEET key                   Delete a key
  PEEK key                   Check if a key exists
  KEYS                       List all keys
  NUKE                       Delete everything (careful!)
  STATS                      Show database statistics
  HELP                       Show this help message

Examples:
  SHOVE greeting "hello world"
  SHOVE session_token "abc123" TTL 3600
  YOINK greeting
  YEET greeting
  PEEK greeting`

	return Result{Ok: true, Message: help}
}

// turning results into text for the TCP folks. 
// mostly just printing strings with some fancy stats formatting.
func (r Result) FormatText() string {
	var sb strings.Builder

	if !r.Ok {
		sb.WriteString("ERR: ")
		sb.WriteString(r.Message)
		return sb.String()
	}

	if r.Value != "" {
		sb.WriteString(r.Value)
		return sb.String()
	}

	sb.WriteString(r.Message)

	if r.Data != nil {
		switch data := r.Data.(type) {
		case []string:
			sb.WriteString("\n")
			for i, s := range data {
				sb.WriteString(fmt.Sprintf("  %d) %s\n", i+1, s))
			}
		case storage.EngineStats:
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("  Keys:       %d\n", data.KeyCount))
			sb.WriteString(fmt.Sprintf("  Tree Depth: %d\n", data.TreeDepth))
			sb.WriteString(fmt.Sprintf("  Pages:      %d\n", data.PageCount))
			sb.WriteString(fmt.Sprintf("  File Size:  %d bytes\n", data.FileSize))
			sb.WriteString(fmt.Sprintf("  Puts:       %d\n", data.PutCount))
			sb.WriteString(fmt.Sprintf("  Gets:       %d\n", data.GetCount))
			sb.WriteString(fmt.Sprintf("  Deletes:    %d\n", data.DeleteCount))
			sb.WriteString(fmt.Sprintf("  Uptime:     %s\n", data.Uptime))
		default:
			jsonBytes, _ := json.MarshalIndent(data, "  ", "  ")
			sb.WriteString("\n  ")
			sb.WriteString(string(jsonBytes))
		}
	}

	return sb.String()
}

// JSON for the web UI. everyone loves JSON.
func (r Result) FormatJSON() []byte {
	data, _ := json.Marshal(r)
	return data
}
