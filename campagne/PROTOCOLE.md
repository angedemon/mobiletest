# Campagne comparative Yélé / nPerf (ISS-05)

Objectif : chiffrer les écarts entre Yélé et nPerf, les expliquer, et vérifier
qu'ils restent dans une tolérance fixée **avant** la campagne.

## Tolérance (à valider avant de commencer)

| Mesure | Tolérance | Réglage |
|---|---|---|
| Débit descendant et montant | ±15 % | `TOL_DEBIT_PCT` |
| Latence | ±15 ms | `TOL_LATENCE_MS` |
| Validation de la campagne | 80 % des paires dans la tolérance | `TOL_TAUX_PAIRES` |

Ces valeurs sont en tête de `comparer_nperf.py`. Les modifier après avoir vu
les résultats fausserait la campagne.

## Conditions identiques

Pour chaque point de mesure :

1. Même téléphone, même SIM, même opérateur, données mobiles (Wi-Fi coupé).
2. Même endroit, immobile, à l'extérieur ou près d'une fenêtre.
3. Tests enchaînés : nPerf puis Yélé, puis Yélé puis nPerf au point suivant,
   pour ne pas avantager l'une des applications.
4. Aucune autre application ne consomme de données (mises à jour coupées).
5. Dans nPerf, noter le serveur choisi. Dans Yélé, le serveur est affiché sous
   la jauge.

Viser au moins **30 points**, répartis sur les opérateurs, les technologies
(3G, 4G, 5G) et les moments de la journée (heures creuses et de pointe).

## Saisie et analyse

1. Saisir chaque test nPerf dans une copie de `releves_nperf_modele.csv`
   (séparateur point-virgule ; date JJ/MM/AAAA, heure HH:MM en heure locale).
2. Exporter les mesures Yélé de la période : tableau de bord → Historique →
   filtrer les dates → **Exporter CSV**.
3. Lancer :

   ```bash
   python comparer_nperf.py yele-mesures-AAAA-MM-JJ.csv releves_nperf.csv rapport.md
   ```

Le script apparie chaque test nPerf au test Yélé du même opérateur fait à
moins de 10 minutes et 200 m, puis calcule les écarts et le taux de paires
dans la tolérance.

## Causes d'écart connues

| Cause | Effet attendu | Comment la vérifier |
|---|---|---|
| **Serveur** : nPerf choisit un serveur proche, souvent chez l'opérateur ; Yélé mesure jusqu'à son serveur (Render, hors d'Afrique, tant qu'il n'est pas déplacé) | Latence Yélé nettement plus haute, débits Yélé plus bas en 4G+/5G | Comparer les points où nPerf a utilisé un serveur étranger |
| **Nombre de connexions** : nPerf ouvre plus de flux parallèles | Débits élevés sous-estimés par Yélé (au-delà de ~50 Mb/s) | Écart croissant avec le débit |
| **Durée des phases** : réglable dans Yélé (5/10/15 s) | Phases courtes : débit plus bas (montée en charge TCP) | Refaire quelques points en 15 s |
| **Méthode de latence** : HTTP (Yélé) vs TCP/UDP (nPerf) | Latence Yélé de quelques ms plus haute | Écart constant, indépendant du lieu |
| **Variabilité du réseau** entre deux tests successifs | Écarts aléatoires des deux côtés | Écart médian proche de 0, dispersion forte |

Le levier principal est le serveur : héberger le serveur de mesure au Burkina
Faso (voir « Héberger le serveur de mesure en Afrique » dans le README)
devrait ramener la latence et les débits élevés au niveau de nPerf.
