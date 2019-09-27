package iso8583

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
)

type FileSpec struct {
	XMLName  xml.Name      `xml:"iso8583"`
	Elements []ElementSpec `xml:"bit"`
}

type ElementSpec struct {
	XMLName    xml.Name `xml:"bit"`
	Pos        int      `xml:"pos,attr"`
	DataType   string   `xml:"type,attr"`
	LengthType string   `xml:"varlen,attr"`
	MaxLength  int      `xml:"maxlen,attr"`
}

func LoadSpecFromFile(file string) (map[int]ElementSpec, error) {
	xmlFile, err := os.Open(file)
	// if we os.Open returns an error then handle it
	if err != nil {
		return nil, err
	}

	fmt.Printf("Successfully Opened %s\n", file)
	// defer the closing of our xmlFile so that we can parse it later on
	defer xmlFile.Close()

	// read our opened xmlFile as a byte array.
	byteValue, _ := ioutil.ReadAll(xmlFile)

	// we initialize our Users array
	var fileSpec FileSpec
	// we unmarshal our byteArray which contains our
	// xmlFiles content into 'users' which we defined above
	err = xml.Unmarshal(byteValue, &fileSpec)
	if err != nil {
		return nil, err
	}

	// we iterate through every user within our users array and
	// print out the user Type, their name, and their facebook url
	// as just an example
	specs := make(map[int]ElementSpec)
	for i := 0; i < len(fileSpec.Elements); i++ {
		specs[fileSpec.Elements[i].Pos] = fileSpec.Elements[i]
	}

	return specs, nil
}
