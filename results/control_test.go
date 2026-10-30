package results

import "testing"

func TestIsControlAgent(t *testing.T) {
	agents := " Chef@ARCEP.bf, agent1@arcep.bf ,"
	for email, want := range map[string]bool{
		"chef@arcep.bf":       true,
		"  AGENT1@arcep.bf ":  true,
		"citoyen@exemple.bf":  false,
		"":                    false,
	} {
		if got := isControlAgent(email, agents); got != want {
			t.Errorf("isControlAgent(%q) = %v, attendu %v", email, got, want)
		}
	}
	if isControlAgent("chef@arcep.bf", "") {
		t.Error("liste vide : personne n'est agent")
	}
}

func TestParseControlPayload(t *testing.T) {
	ok := `{"campaign":"OUAGA-2026-11","deviceId":"dev-1","seq":3,
		"measuredAt":"2026-11-03T09:15:00Z","latitude":12.37,"longitude":-1.52,
		"simOperator":"Orange BF","dl":18.4}`
	p, problem := parseControlPayload([]byte(ok))
	if p == nil {
		t.Fatalf("mesure valide refusée : %s", problem)
	}
	if p.Campaign != "OUAGA-2026-11" || p.Seq != 3 || p.SimOperator != "Orange BF" {
		t.Errorf("lecture incorrecte : %+v", p)
	}

	for name, raw := range map[string]string{
		"json":       `{`,
		"campagne":   `{"campaign":"ouaga 2026","deviceId":"d","seq":1,"measuredAt":"2026-11-03T09:15:00Z"}`,
		"téléphone":  `{"campaign":"C1","deviceId":"","seq":1,"measuredAt":"2026-11-03T09:15:00Z"}`,
		"cycle":      `{"campaign":"C1","deviceId":"d","seq":0,"measuredAt":"2026-11-03T09:15:00Z"}`,
		"date":       `{"campaign":"C1","deviceId":"d","seq":1,"measuredAt":"03/11/2026"}`,
		"position":   `{"campaign":"C1","deviceId":"d","seq":1,"measuredAt":"2026-11-03T09:15:00Z","latitude":120}`,
	} {
		if p, _ := parseControlPayload([]byte(raw)); p != nil {
			t.Errorf("%s : mesure invalide acceptée", name)
		}
	}
}
