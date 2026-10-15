package mongodb

import (
	"testing"

	"github.com/librespeed/speedtest-go/database/schema"
)

func TestRankBelow(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := rankBelow(values, 8.5); got != 80 {
		t.Errorf("rankBelow(8.5) = %v, attendu 80", got)
	}
	if got := rankBelow(nil, 5); got != 0 {
		t.Errorf("rankBelow sans données = %v, attendu 0", got)
	}
}

func TestRegexpQuote(t *testing.T) {
	if got := regexpQuote("Orange (BF).*"); got != `Orange \(BF\)\.\*` {
		t.Errorf("regexpQuote = %q", got)
	}
}

func TestExportColumnsMatchValues(t *testing.T) {
	if n, c := len(ExportRow{}.Values()), len(ExportColumns); n != c {
		t.Fatalf("%d valeurs pour %d colonnes", n, c)
	}
}

func TestToDocumentTestIDAndData(t *testing.T) {
	m := &MongoDB{}
	doc := m.toDocument(&schema.TelemetryData{
		Extra: `{"type":"active","testId":"abc","deviceId":"d1","dataUsedKiB":2048}`,
	})
	if doc.TestID != "abc" || doc.DeviceID != "d1" || doc.DataUsedKiB != 2048 {
		t.Errorf("TestID=%q DeviceID=%q DataUsedKiB=%d", doc.TestID, doc.DeviceID, doc.DataUsedKiB)
	}
	old := m.toDocument(&schema.TelemetryData{Extra: `{"type":"active"}`})
	if old.DataUsedKiB != -1 {
		t.Errorf("données absentes : %d, attendu -1", old.DataUsedKiB)
	}
}
