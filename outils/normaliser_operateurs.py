"""Migration unique : ramène les noms d'opérateurs déjà en base à leur libellé
canonique (mêmes règles que NormalizeOperator dans database/mongodb/operators.go).

Les nouvelles mesures sont normalisées à l'enregistrement par le serveur ; ce
script ne sert qu'aux documents antérieurs (dont les 786 mesures importées).

Usage :
    pip install pymongo
    set SPEEDTEST_DATABASE_CONNECTION_STRING=mongodb+srv://...
    python normaliser_operateurs.py              # simulation : affiche les changements
    python normaliser_operateurs.py --appliquer  # écrit en base

Le script est idempotent : le relancer après coup ne modifie plus rien.
"""

import os
import sys

from pymongo import MongoClient

# Liste identique à operatorAliases (Go) et à OPERATOR_ALIASES (AI_yele/model.py).
OPERATOR_ALIASES = [
    ("onatel", "ONATEL / Telmob"),
    ("office national des telecommunications", "ONATEL / Telmob"),
    ("telmob", "ONATEL / Telmob"),
    ("orange burkina", "Orange Burkina Faso"),
    ("anptic", "ANPTIC"),
    ("agence nationale de promotion des tic", "ANPTIC"),
    ("autorite de regulation des communications", "ARCEP Burkina"),
    ("telecel", "Telecel Faso"),
    ("moov", "Moov Africa"),
]

CHAMPS = ["operator", "sim_operator"]


def canon_operator(name):
    """Ramène chaque variante d'écriture au libellé canonique."""
    if not isinstance(name, str) or not name:
        return name
    low = name.lower()
    for motif, canonique in OPERATOR_ALIASES:
        if motif in low:
            return canonique
    return name.strip()


def main():
    appliquer = "--appliquer" in sys.argv[1:]
    uri = os.environ.get("SPEEDTEST_DATABASE_CONNECTION_STRING")
    if not uri:
        sys.exit("SPEEDTEST_DATABASE_CONNECTION_STRING manquante")
    db = os.environ.get("SPEEDTEST_DATABASE_NAME", "yele_speedtest")
    collection = MongoClient(uri)[db]["speedtest_results"]

    total = 0
    for champ in CHAMPS:
        for valeur in collection.distinct(champ):
            canonique = canon_operator(valeur)
            if canonique == valeur:
                continue
            n = collection.count_documents({champ: valeur})
            total += n
            print(f"{champ}: {valeur!r} -> {canonique!r} ({n} documents)")
            if appliquer:
                collection.update_many({champ: valeur}, {"$set": {champ: canonique}})

    if not total:
        print("Rien à normaliser.")
    elif appliquer:
        print(f"{total} champs mis à jour.")
    else:
        print(f"{total} champs à mettre à jour. Relancer avec --appliquer pour écrire.")


if __name__ == "__main__":
    main()
