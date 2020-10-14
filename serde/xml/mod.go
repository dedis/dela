// Package xml implements the context engine for the XML encoding.
//
// Documentation Last Review: 14.10.2020
//
package xml

import (
	"encoding/xml"

	"go.dedis.ch/dela/serde"
)

// xmlEngine is a context engine that uses the XML encoding. See encoding/xml.
//
// - implements serde.ContextEngine
type xmlEngine struct{}

// NewContext returns a new serde context that is using the XML encoding.
func NewContext() serde.Context {
	return serde.NewContext(xmlEngine{})
}

// GetFormat implements serde.ContextEngine. It returns the XML format
// identifier.
func (xmlEngine) GetFormat() serde.Format {
	return serde.FormatXML
}

// Marshal implements serde.ContextEngine. It marshals the message using the XML
// encoding.
func (xmlEngine) Marshal(m interface{}) ([]byte, error) {
	return xml.Marshal(m)
}

// Unmarshal implements serde.ContextEngine. It unmarshals the data into the
// message using the XML encoding.
func (xmlEngine) Unmarshal(data []byte, m interface{}) error {
	return xml.Unmarshal(data, m)
}
