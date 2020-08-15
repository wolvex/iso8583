package iso8583

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
)

type StringPackager struct {
	Specs  map[int]ElementSpec
	Reader *bufio.Reader
	Writer *bufio.Writer
}

func NewStringPackager(specs map[int]ElementSpec, reader *bufio.Reader, writer *bufio.Writer) *StringPackager {
	packager := &StringPackager{
		Specs:  specs,
		Reader: reader,
		Writer: writer,
	}
	return packager
}

func ReadNextBytes(reader *bufio.Reader, length int) ([]byte, error) {
	log.WithField("length", length).Info("Reading next X bytes from stream")

	//payload := make([]byte, 0)
	var payload []byte
	for {
		if len(payload) >= length {
			return payload, nil
		}
		if b, err := reader.ReadByte(); err != nil {
			if payload != nil {
				log.WithFields(log.Fields{
					"payload": string(payload),
					"error":   err,
				}).Error("Error caught but buffer has data")
			}
			return payload, err
		} else {
			//log.WithFields(log.Fields{
			//	"byte":    b,
			//	"payload": payload,
			//}).Info("Reading payload")
			payload = append(payload, b)
		}
	}
	//return payload, nil
}

func (packager *StringPackager) Read() (msg []byte, err error) {
	/**
	header := make([]byte, 4)
	_, err = io.ReadFull(packager.Reader, header)
	if err != nil {
		return
	}*/
	header, err := ReadNextBytes(packager.Reader, 4)
	if err != nil {
		return nil, err
	}
	log.WithField("header", string(header)).Info("Read 4 bytes of header")

	size := 0
	size, err = strconv.Atoi(string(header[:]))
	if err != nil {
		return
	}

	/**
	body := make([]byte, len)
	_, err = io.ReadFull(packager.Reader, body)
	if err != nil {
		return
	}
	*/
	body, err := ReadNextBytes(packager.Reader, size)
	if err != nil {
		return
	}
	log.WithField("body", string(body)).Info("Read X bytes of body")

	return body, nil
}

func (packager *StringPackager) Send(msg *IsoMsg) (err error) {
	payload := ""
	payload, err = packager.Pack(msg)
	if err != nil {
		return
	}

	length := len(payload)
	varlen := fmt.Sprintf("0000%d", length)
	payload = fmt.Sprintf("%s%s", varlen[len(varlen)-4:], payload)

	if _, err = packager.Writer.Write([]byte(payload)); err != nil {
		return
	} else {
		err = packager.Writer.Flush()
	}

	return
}

func (packager *StringPackager) Pack(msg *IsoMsg) (payload string, err error) {
	primary := ""
	secondary := ""
	payload = ""
	for i := 2; i <= 128; i++ {
		flag := "0"
		if msg.GetBit(i) != "" {
			flag = "1"
			spec, ok := packager.Specs[i]
			if !ok {
				err = fmt.Errorf("Unable to find specification for element %d", i)
				return
			}

			switch spec.LengthType {
			case "2": //llvar
				val := fmt.Sprintf("%02d", len(msg.GetBit(i)))
				payload = fmt.Sprintf("%s%s%s", payload, val, msg.GetBit(i))
				break

			case "3": //lllvar
				val := fmt.Sprintf("%03d", len(msg.GetBit(i)))
				payload = fmt.Sprintf("%s%s%s", payload, val, msg.GetBit(i))
				break

			default: //fixedlen
				val := msg.GetBit(i)
				switch spec.DataType {
				case "string":
					for len(val) < spec.MaxLength {
						val = fmt.Sprintf("%s ", val)
					}
					break
				default:
					for len(val) < spec.MaxLength {
						val = fmt.Sprintf("0%s", val)
					}
					break
				}
				payload = fmt.Sprintf("%s%s", payload, val)
			}

		}

		if i < 65 {
			primary = fmt.Sprintf("%s%s", primary, flag)
		} else {
			secondary = fmt.Sprintf("%s%s", secondary, flag)
		}
	}

	if secondary != "" {
		secondary, err = binToHex(secondary)
		if err != nil {
			return "", err
		}
		primary = fmt.Sprintf("1%s", primary)
	} else {
		primary = fmt.Sprintf("0%s", primary)
	}

	primary, err = binToHex(primary)
	if err != nil {
		return
	}

	if secondary != "" {
		payload = fmt.Sprintf("%s%s%s%s", msg.GetMessageType(), primary, secondary, payload)
	} else {
		payload = fmt.Sprintf("%s%s%s", msg.GetMessageType(), primary, payload)
	}

	return
}

func (packager *StringPackager) Unpack(msg []byte) (iso *IsoMsg, err error) {
	str := string(msg[:])
	if len(str) < 20 {
		return nil, nil
	}

	iso = NewIsoMsg()

	m, pos := readNext(str, 0, 4) //get next 4 chars
	iso.SetMessageType(m)

	bitmap := ""
	m, pos = readNext(str, pos, 16) //get next 16 chars
	bitmap, err = hexToBin(m)
	if err != nil {
		return
	}

	//populate primary bitmaps
	for i := 1; i <= 64; i++ {
		if bitmap[i-1:i] == "1" {
			spec, ok := packager.Specs[i]
			length := 0

			if !ok {
				err = fmt.Errorf("Unable to get specification for element %d", i)
				return
			}

			switch spec.LengthType {
			case "2":
				//get next 2 chars
				m, pos = readNext(str, pos, 2)
				length, err = strconv.Atoi(m)
				if err != nil {
					return
				}
				break

			case "3":
				//get next 3 chars
				m, pos = readNext(str, pos, 3)
				length, err = strconv.Atoi(m)
				if err != nil {
					return
				}
				break

			default:
				length = spec.MaxLength
				break
			}

			//get next 'length' chars
			m, pos = readNext(str, pos, length)
			iso.SetBit(i, m)
		}
	}

	if iso.GetBit(1) == "" {
		return
	}

	//populate secondary bitmaps
	bitmap, err = hexToBin(iso.GetBit(1))
	if err != nil {
		return
	}
	for i := 65; i <= 128; i++ {
		//fmt.Printf("processing bit %d\n", i)
		if bitmap[i-65:i-64] == "1" {
			spec, ok := packager.Specs[i]
			length := 0

			if !ok {
				err = fmt.Errorf("Unable to get specification for element %d", i)
				return
			}

			switch spec.LengthType {
			case "2":
				//get next 2 chars
				m, pos = readNext(str, pos, 2)
				length, err = strconv.Atoi(m)
				if err != nil {
					return
				}
				break

			case "3":
				//get next 3 chars
				m, pos = readNext(str, pos, 3)
				length, err = strconv.Atoi(m)
				if err != nil {
					return
				}
				break

			default:
				length = spec.MaxLength
				break
			}

			//get next 'length' chars
			m, pos = readNext(str, pos, length)
			iso.SetBit(i, m)
		}
	}
	return
}

func hexToBin(hex string) (string, error) {
	ui, err := strconv.ParseUint(hex, 16, 64)
	if err != nil {
		return "", err
	}

	bin := fmt.Sprintf("%016b", ui)
	for {
		if len(bin) >= 64 {
			break
		}
		bin = fmt.Sprintf("0%s", bin)
	}
	// %016b indicates base 2, zero padded, with 16 characters
	return bin, nil
}

func binToHex(s string) (string, error) {
	ui, err := strconv.ParseUint(s, 2, 64)
	if err != nil {
		return "", err
	}

	hex := strings.ToUpper(fmt.Sprintf("%x", ui))
	for {
		if len(hex) >= 16 {
			break
		}
		hex = fmt.Sprintf("0%s", hex)
	}
	return hex, nil
}

func readNext(str string, start, length int) (s string, i int) {
	s = str[start : start+length]
	i = start + length
	return
}
