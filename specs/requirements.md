# Requirements - stabsight

## 1. Vision du Produit
Un agent d'observabilité réseau autonome et moderne, conçu pour diagnostiquer finement les dégradations de connectivité. Il dépasse le simple "ping" en corrélant plusieurs types de mesures (latence de bout en bout, sauts de routage, tests de bande passante) et s'intègre nativement dans les pipelines de télémétrie actuels (OpenTelemetry).

## 2. Exigences Fonctionnelles (Functional Requirements)
*   **FR1 - Configuration :** L'agent doit lire un fichier `config.yaml` définissant les cibles (IP/Domaines) et les sondes à exécuter avec leurs intervalles respectifs.
*   **FR2 - Sondes de latence et disponibilité :** L'agent doit implémenter des sondes ICMP, TCP, DNS, et HTTP.
*   **FR3 - Sondes de diagnostic et bande passante :** L'agent doit implémenter une sonde MTR (Traceroute) et une sonde Speedtest (Download/Upload).
*   **FR4 - Ordonnancement intelligent :** L'exécution d'une sonde Speedtest doit obligatoirement suspendre toutes les autres sondes actives pour éviter le phénomène de bufferbloat, et les reprendre une fois terminée.
*   **FR5 - Calcul du Jitter :** L'agent doit calculer le jitter en temps réel (différence absolue entre deux RTT consécutifs) sans lisser les données.
*   **FR6 - Diagnostic croisé :** Le système doit être capable de tagger des anomalies en corrélant une perte de paquets globale avec un saut spécifique via la sonde MTR.
*   **FR7 - Interface CLI et surcharge de configuration :** L'agent doit exposer une interface en ligne de commande (via `cobra`) et charger sa configuration via `viper`. Les réglages opérationnels (niveau de log, endpoint OTLP, adresse d'écoute `/metrics`, chemin du fichier de config) doivent pouvoir être surchargés par variable d'environnement (préfixe `INTERNET_MONITOR_`) en plus du flag CLI et du fichier YAML. Cette surcharge par environnement ne s'applique jamais à la définition des cibles/sondes (`targets:`), qui reste régie exclusivement par FR1.

## 3. Exigences Techniques (Non-Functional Requirements)
*   **NFR1 - Langage :** Développé en Go (version 1.21 ou supérieure).
*   **NFR2 - Télémétrie :** Utilisation stricte du SDK OpenTelemetry (OTLP) pour Go.
*   **NFR3 - Format des métriques :** Utilisation des **Exponential/Native Histograms** pour la latence. Les métriques doivent être exportables en push (gRPC/HTTP OTLP) et en pull (endpoint HTTP `/metrics` Prometheus).
*   **NFR4 - Performance :** Les sondes doivent s'exécuter de manière concurrente (Goroutines) avec une empreinte CPU et RAM minimale. Aucune requête réseau ne doit être interrompue brutalement lors d'une demande de mise en pause.
*   **NFR5 - Déploiement :** Compilation sous forme d'un binaire statique unique (amd64, arm64) et conteneurisation légère (image base Alpine ou Distroless).
*   **NFR6 - Agnosticité :** L'agent expose uniquement son propre format de nommage de métriques. La rétrocompatibilité (ex: Blackbox Exporter) n'est pas gérée dans le code, mais déléguée au collecteur externe.
*   **NFR7 - CLI et configuration :** L'interface en ligne de commande doit être implémentée avec `spf13/cobra` et le chargement/fusion de configuration avec `spf13/viper` (priorité : flag > variable d'environnement > fichier YAML > défaut). Aucune autre librairie de parsing de flags ou de config (`flag` standard, `envconfig`, etc.) ne doit être introduite en parallèle.
