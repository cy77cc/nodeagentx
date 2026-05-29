package syslog

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
)

// Message represents a parsed syslog message.
type Message struct {
	Facility  int
	Severity  int
	Timestamp time.Time
	Hostname  string
	AppName   string
	ProcID    string
	MsgID     string
	Message   string
}

// decodePriority splits a syslog priority value into facility and severity.
func decodePriority(pri int) (facility, severity int) {
	return pri / 8, pri % 8
}

// Parse parses a raw syslog message (RFC 5424 or RFC 3164 format).
func Parse(data []byte) (*Message, error) {
	if len(data) == 0 || data[0] != '<' {
		return nil, fmt.Errorf("syslog: missing priority")
	}
	end := bytes.IndexByte(data, '>')
	if end < 0 {
		return nil, fmt.Errorf("syslog: unclosed priority")
	}
	pri, err := strconv.Atoi(string(data[1:end]))
	if err != nil {
		return nil, fmt.Errorf("syslog: invalid priority: %w", err)
	}

	msg := &Message{}
	msg.Facility, msg.Severity = decodePriority(pri)

	rest := data[end+1:]
	if len(rest) == 0 {
		return nil, fmt.Errorf("syslog: empty message after priority")
	}

	// Try RFC 5424: starts with version number (e.g., "1 ")
	if rest[0] >= '0' && rest[0] <= '9' {
		return parseRFC5424(rest, msg)
	}
	return parseRFC3164(rest, msg)
}

// parseRFC5424 parses an RFC 5424 syslog message.
// Format: VERSION SP TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP STRUCTURED-DATA SP MSG
func parseRFC5424(data []byte, msg *Message) (*Message, error) {
	space := bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing version")
	}
	// Skip version
	data = data[space+1:]

	// Timestamp
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing timestamp")
	}
	ts, err := time.Parse(time.RFC3339Nano, string(data[:space]))
	if err == nil {
		msg.Timestamp = ts
	}
	data = data[space+1:]

	// Hostname
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing hostname")
	}
	msg.Hostname = string(data[:space])
	data = data[space+1:]

	// AppName
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing appname")
	}
	msg.AppName = string(data[:space])
	data = data[space+1:]

	// ProcID
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing procid")
	}
	msg.ProcID = string(data[:space])
	data = data[space+1:]

	// MsgID
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing msgid")
	}
	msg.MsgID = string(data[:space])
	data = data[space+1:]

	// Structured data (skip until space)
	if len(data) > 0 && data[0] == '-' {
		data = data[1:]
		if len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
	} else {
		// Skip structured data (bracketed)
		if idx := bytes.IndexByte(data, ']'); idx >= 0 {
			data = data[idx+1:]
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
		}
	}

	msg.Message = string(data)
	return msg, nil
}

// parseRFC3164 parses an RFC 3164 syslog message.
// Format: TIMESTAMP SP HOSTNAME SP APP-NAME[PID]: SP MSG
func parseRFC3164(data []byte, msg *Message) (*Message, error) {
	if len(data) < 15 {
		return nil, fmt.Errorf("syslog 3164: too short")
	}

	// Try to parse timestamp (first 15 chars: "Jan  2 15:04:05")
	ts, err := time.Parse("Jan  2 15:04:05", string(data[:15]))
	if err == nil {
		msg.Timestamp = ts
		data = data[16:] // skip timestamp + space
	} else {
		// Try single-digit day: "Jan 2 15:04:05"
		ts, err = time.Parse("Jan 2 15:04:05", string(data[:14]))
		if err == nil {
			msg.Timestamp = ts
			data = data[15:]
		} else {
			return nil, fmt.Errorf("syslog 3164: cannot parse timestamp")
		}
	}

	// Hostname
	space := bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 3164: missing hostname")
	}
	msg.Hostname = string(data[:space])
	data = data[space+1:]

	// App-Name[PID]: or App-Name:
	colon := bytes.IndexByte(data, ':')
	if colon < 0 {
		msg.Message = string(data)
		return msg, nil
	}
	appPart := data[:colon]
	data = data[colon+1:]

	// Skip leading space in message
	if len(data) > 0 && data[0] == ' ' {
		data = data[1:]
	}

	// Check for PID in brackets
	if idx := bytes.IndexByte(appPart, '['); idx >= 0 {
		msg.AppName = string(appPart[:idx])
		// Extract PID
		end := bytes.IndexByte(appPart[idx:], ']')
		if end >= 0 {
			msg.ProcID = string(appPart[idx+1 : idx+end])
		}
	} else {
		msg.AppName = string(appPart)
	}

	msg.Message = string(data)
	return msg, nil
}
