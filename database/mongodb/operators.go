package mongodb

import "strings"

// operatorAliases ramène les différentes écritures d'un même opérateur à un
// libellé unique. Une variante est reconnue dès qu'elle contient le motif
// (comparaison insensible à la casse) ; le premier motif trouvé l'emporte.
//
// Liste identique à OPERATOR_ALIASES du service IA (AI_yele/model.py) et au
// script outils/normaliser_operateurs.py : toute modification doit être
// reportée aux trois endroits.
var operatorAliases = []struct {
	motif     string
	canonique string
}{
	{"onatel", "ONATEL / Telmob"},
	{"office national des telecommunications", "ONATEL / Telmob"},
	{"telmob", "ONATEL / Telmob"},
	{"orange burkina", "Orange Burkina Faso"},
	{"anptic", "ANPTIC"},
	{"agence nationale de promotion des tic", "ANPTIC"},
	{"autorite de regulation des communications", "ARCEP Burkina"},
	{"telecel", "Telecel Faso"},
	{"moov", "Moov Africa"},
}

// NormalizeOperator renvoie le libellé canonique d'un nom d'opérateur, par
// exemple « Agence Nationale de Promotion des TIC (ANPTIC) » -> « ANPTIC ».
// Un nom inconnu est conservé, débarrassé de ses espaces superflus.
func NormalizeOperator(name string) string {
	if name == "" {
		return name
	}
	low := strings.ToLower(name)
	for _, a := range operatorAliases {
		if strings.Contains(low, a.motif) {
			return a.canonique
		}
	}
	return strings.TrimSpace(name)
}
