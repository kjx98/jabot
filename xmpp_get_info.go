package jabot

import (
	"fmt"
	"time"
)

func (c *Jabot) RawVersion(from, to, id, version, osName string) error {
	body := "<name>go-xmpp</name><version>" + version + "</version><os>" +
		osName + "</os>"
	_, err := c.client.RawInformationQuery(from, to, id, "result", "jabber:iq:version",
		body)
	return err
}

func (c *Jabot) RawLast(from, to, id string, last int) error {
	body := fmt.Sprintf("<query xmlns='jabber:iq:last' "+
		"seconds='%d'>Working</query>", last)
	_, err := c.client.RawInformation(from, to, id, "result", body)
	return err
}

func (c *Jabot) RawLastNA(from, to, id string) error {
	body := fmt.Sprintf("<error type='cancel'><service-unavailable " +
		"xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/></error>")
	_, err := c.client.RawInformation(from, to, id, "error", body)
	return err
}

func (c *Jabot) RawIQtime(from, to, id string) error {
	tt := time.Now()
	zone, _ := tt.Zone()
	body := fmt.Sprintf("<time xmlns='urn:xmpp:time'>\n<tzo>%s</tzo><utc>%s"+
		"</utc></time>", zone, tt.UTC().Format("2006-01-02T15:03:04Z"))
	_, err := c.client.RawInformation(from, to, id, "result", body)
	return err
}
