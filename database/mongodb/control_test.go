package mongodb

import "testing"

func chain(device string, payloads ...string) []ControlMeasurement {
	var out []ControlMeasurement
	prev := ""
	for i, p := range payloads {
		h := ChainHash(prev, p)
		out = append(out, ControlMeasurement{DeviceID: device, Seq: int64(i + 1), Payload: p, PrevHash: prev, Hash: h})
		prev = h
	}
	return out
}

func TestChainHashDependsOnPrevious(t *testing.T) {
	a := ChainHash("", `{"seq":1}`)
	if a != ChainHash("", `{"seq":1}`) {
		t.Fatal("même entrée : empreintes différentes")
	}
	if a == ChainHash("autre", `{"seq":1}`) {
		t.Error("l'empreinte doit dépendre de la mesure précédente")
	}
	if len(a) != 64 {
		t.Errorf("empreinte de %d caractères, attendu 64 (SHA-256)", len(a))
	}
}

func TestVerifyControlChain(t *testing.T) {
	list := append(chain("orange", `{"seq":1}`, `{"seq":2}`, `{"seq":3}`),
		chain("telecel", `{"seq":1}`, `{"seq":2}`)...)
	if broken := VerifyControlChain(list); len(broken) != 0 {
		t.Fatalf("chaîne intacte signalée cassée : %d", len(broken))
	}

	// Mesure modifiée après coup : la chaîne casse sur cette mesure.
	modifiee := append([]ControlMeasurement(nil), list...)
	modifiee[1].Payload = `{"seq":2,"dl":99}`
	if broken := VerifyControlChain(modifiee); len(broken) != 1 || broken[0].Seq != 2 {
		t.Errorf("modification non détectée : %+v", broken)
	}

	// Mesure supprimée : la suivante ne se raccroche plus.
	supprimee := append([]ControlMeasurement{list[0]}, list[2:]...)
	if broken := VerifyControlChain(supprimee); len(broken) != 1 || broken[0].Seq != 3 {
		t.Errorf("suppression non détectée : %+v", broken)
	}
}
