package iso8583

import (
	"encoding/json"
	"errors"
	"strconv"
)

type IsoMsg struct {
	MessageType string `json:"messageType"`
	//PrimaryBitmap   string         `json:"primaryBitmap"`
	//SecondaryBitmap string         `json:"secondaryBitmap"`
	Element map[int]string `json:"elements"`
}

func NewIsoMsg() *IsoMsg {
	elements := make(map[int]string)
	return &IsoMsg{
		Element: elements,
	}
}

func (msg *IsoMsg) SetMessageType(val string) {
	msg.MessageType = val
}

func (msg *IsoMsg) GetMessageType() string {
	return msg.MessageType
}

/*
func (msg *IsoMsg) GetPrimary() string {
	return msg.PrimaryBitmap
}

func (msg *IsoMsg) GetSecondary() string {
	return msg.SecondaryBitmap
}
*/

func (msg *IsoMsg) SetBit(pos int, val string) {
	msg.Element[pos] = val
}

func (msg *IsoMsg) GetBit(pos int) string {
	if val, ok := msg.Element[pos]; ok {
		return val
	} else {
		return ""
	}
}

func (msg *IsoMsg) GetRespCode() (int, error) {
	if val, ok := msg.Element[39]; ok {
		return strconv.Atoi(val)
	} else {
		return -255, errors.New("Response code not found")
	}
}

func (msg *IsoMsg) GetSlice(pos, start, length int) string {
	if val, ok := msg.Element[pos]; ok {
		end := start + length
		if start >= len(val) || start < 0 || end > len(val) {
			return ""
		} else {
			return val[start:end]
		}
	} else {
		return ""
	}
}

func (msg *IsoMsg) Dump() string {
	m, _ := json.Marshal(msg)
	return string(m)
}
