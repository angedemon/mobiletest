"""Campagne comparative Yélé / nPerf (ISS-05).

Apparie chaque test nPerf au test Yélé le plus proche fait au même endroit,
au même moment, avec le même opérateur, puis calcule les écarts et le taux
de paires dans la tolérance définie avant la campagne.

Usage :
    python comparer_nperf.py yele-mesures.csv releves_nperf.csv [rapport.md]

- yele-mesures.csv : export CSV du tableau de bord (Historique → Exporter CSV)
- releves_nperf.csv : relevés nPerf saisis dans le modèle releves_nperf_modele.csv

Bibliothèque standard uniquement : aucune installation nécessaire.
"""

import csv
import math
import statistics
import sys
from datetime import datetime, timezone

# ── Tolérances fixées AVANT la campagne (voir PROTOCOLE.md) ────────────────
TOL_DEBIT_PCT = 15.0      # écart relatif admis sur les débits
TOL_LATENCE_MS = 15.0     # écart absolu admis sur la latence
TOL_TAUX_PAIRES = 80.0    # % de paires dans la tolérance pour valider

# ── Appariement ───────────────────────────────────────────────────────────
MAX_ECART_MIN = 10        # tests faits à moins de 10 minutes d'intervalle
MAX_DISTANCE_M = 200      # et à moins de 200 m l'un de l'autre


def lire_csv(path):
    with open(path, encoding="utf-8-sig", newline="") as f:
        sample = f.read(4096)
        f.seek(0)
        dialect = csv.Sniffer().sniff(sample, delimiters=";,")
        return list(csv.DictReader(f, dialect=dialect))


def nombre(v):
    try:
        return float(str(v).replace(",", "."))
    except (TypeError, ValueError):
        return None


def date_yele(v):
    return datetime.fromisoformat(v.replace("Z", "+00:00"))


def date_nperf(row):
    # Heure locale du Burkina Faso = UTC.
    d = datetime.strptime(f"{row['date']} {row['heure']}", "%d/%m/%Y %H:%M")
    return d.replace(tzinfo=timezone.utc)


def distance_m(lat1, lon1, lat2, lon2):
    r = 6_371_000
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp, dl = p2 - p1, math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * r * math.asin(math.sqrt(a))


def meme_operateur(a, b):
    a, b = (a or "").lower(), (b or "").lower()
    return bool(a and b) and (a in b or b in a or a.split()[0] == b.split()[0])


def apparier(yele, nperf):
    paires = []
    libres = list(yele)
    for n in nperf:
        tn = date_nperf(n)
        lat, lon = nombre(n["latitude"]), nombre(n["longitude"])
        meilleur, score = None, None
        for y in libres:
            if not meme_operateur(y["operator"], n["operateur"]):
                continue
            dt = abs((date_yele(y["timestamp"]) - tn).total_seconds()) / 60
            if dt > MAX_ECART_MIN:
                continue
            d = distance_m(lat, lon, nombre(y["latitude"]), nombre(y["longitude"]))
            if d > MAX_DISTANCE_M:
                continue
            s = dt + d / 50  # 1 min ≈ 50 m
            if score is None or s < score:
                meilleur, score = y, s
        if meilleur:
            libres.remove(meilleur)
            paires.append((meilleur, n))
    return paires


def ecart_pct(y, n):
    return None if not n else (y - n) / n * 100


def analyser(paires):
    lignes, dans_tol = [], 0
    for y, n in paires:
        e = {
            "dl": ecart_pct(nombre(y["downloadMbps"]), nombre(n["download_mbps"])),
            "ul": ecart_pct(nombre(y["uploadMbps"]), nombre(n["upload_mbps"])),
            "ping": nombre(y["pingMs"]) - nombre(n["latence_ms"]),
        }
        ok = (abs(e["dl"] or 0) <= TOL_DEBIT_PCT and abs(e["ul"] or 0) <= TOL_DEBIT_PCT
              and abs(e["ping"]) <= TOL_LATENCE_MS)
        dans_tol += ok
        lignes.append((y, n, e, ok))
    return lignes, dans_tol


def rapport(lignes, dans_tol, nb_nperf):
    out = ["# Campagne comparative Yélé / nPerf", ""]
    n = len(lignes)
    if not n:
        return "\n".join(out + ["Aucune paire : vérifiez dates, positions et opérateurs."])
    taux = dans_tol / n * 100
    med = lambda k: statistics.median(e[k] for _, _, e, _ in lignes if e[k] is not None)
    out += [
        f"- Relevés nPerf : {nb_nperf}, paires formées : {n}",
        f"- Tolérance : ±{TOL_DEBIT_PCT:g} % sur les débits, ±{TOL_LATENCE_MS:g} ms sur la latence",
        f"- **Paires dans la tolérance : {dans_tol}/{n} ({taux:.0f} %) — "
        f"{'VALIDÉ' if taux >= TOL_TAUX_PAIRES else 'NON VALIDÉ'}** (seuil {TOL_TAUX_PAIRES:g} %)",
        f"- Écart médian : débit ↓ {med('dl'):+.1f} %, débit ↑ {med('ul'):+.1f} %, "
        f"latence {med('ping'):+.1f} ms",
        "",
        "Un écart médian positif signifie que Yélé mesure plus que nPerf.",
        "",
        "| Date | Opérateur | ↓ Yélé | ↓ nPerf | Écart ↓ | Latence Yélé | Latence nPerf | OK |",
        "|---|---|---|---|---|---|---|---|",
    ]
    for y, nn, e, ok in lignes:
        out.append(
            f"| {nn['date']} {nn['heure']} | {nn['operateur']} | {nombre(y['downloadMbps']):.1f} "
            f"| {nombre(nn['download_mbps']):.1f} | {e['dl']:+.0f} % | {nombre(y['pingMs']):.0f} ms "
            f"| {nombre(nn['latence_ms']):.0f} ms | {'oui' if ok else 'non'} |")
    return "\n".join(out)


def main():
    # Console Windows : sans cela, les flèches et accents font planter print.
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    yele = [r for r in lire_csv(sys.argv[1]) if nombre(r.get("latitude"))]
    nperf = lire_csv(sys.argv[2])
    lignes, ok = analyser(apparier(yele, nperf))
    texte = rapport(lignes, ok, len(nperf))
    if len(sys.argv) > 3:
        with open(sys.argv[3], "w", encoding="utf-8") as f:
            f.write(texte + "\n")
    print(texte)


if __name__ == "__main__":
    main()
