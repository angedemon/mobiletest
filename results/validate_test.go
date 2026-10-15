package results

import (
	"testing"
	"time"
)

func TestValidateTelemetry(t *testing.T) {
	tests := []struct {
		name                   string
		dl, ul, ping, jit, ext string
		wantErr                bool
	}{
		{"test actif de l'app", "25.50", "8.20", "45.00", "3.10",
			`{"type":"active","latitude":12.3656,"longitude":-1.5197,"qoeRating":4,"streamingScore":72.5,"browsingSuccessRate":0.83}`, false},
		{"relève passive", "0", "0", "", "",
			`{"type":"passive","latitude":12.3,"longitude":-1.5,"signalDbm":-95,"signalLevel":2}`, false},
		{"test web interrompu", "", "", "", "", "", false},
		{"débit négatif", "-5", "8", "45", "3", "", true},
		{"débit impossible", "50000", "8", "45", "3", "", true},
		{"débit non numérique", "abc", "8", "45", "3", "", true},
		{"NaN", "NaN", "8", "45", "3", "", true},
		{"extra illisible", "25", "8", "45", "3", "{pas du json", true},
		{"latitude hors du globe", "25", "8", "45", "3", `{"latitude":123,"longitude":-1.5}`, true},
		{"score > 100", "25", "8", "45", "3", `{"streamingScore":150}`, true},
		{"taux > 1", "25", "8", "45", "3", `{"browsingSuccessRate":1.5}`, true},
		{"note QoE > 5", "25", "8", "45", "3", `{"qoeRating":9}`, true},
		{"type inconnu", "25", "8", "45", "3", `{"type":"bot"}`, true},
		{"signal positif", "0", "0", "", "", `{"type":"passive","signalDbm":20}`, true},
		{"radio 4G valide", "25", "8", "45", "3", `{"rsrp":-98,"rsrq":-11,"sinr":12,"simSlot":2}`, false},
		{"RSRP impossible", "25", "8", "45", "3", `{"rsrp":-300}`, true},
		{"min/max et perte", "25", "8", "45", "3", `{"downloadMin":3.2,"downloadMax":41,"pingMin":30,"pingMax":80,"packetLoss":2.5}`, false},
		{"perte > 100 %", "25", "8", "45", "3", `{"packetLoss":140}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTelemetry(tt.dl, tt.ul, tt.ping, tt.jit, tt.ext)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, erreur attendue : %v", err, tt.wantErr)
			}
		})
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	t0 := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4", t0) {
			t.Fatalf("envoi %d refusé alors que la limite est 3", i+1)
		}
	}
	if l.allow("1.2.3.4", t0.Add(time.Second)) {
		t.Error("4e envoi accepté dans la même fenêtre")
	}
	if !l.allow("5.6.7.8", t0.Add(time.Second)) {
		t.Error("une autre IP doit avoir son propre compteur")
	}
	if !l.allow("1.2.3.4", t0.Add(time.Minute)) {
		t.Error("le compteur doit repartir à zéro après la fenêtre")
	}
}
