# Tasks - Plan d'Implémentation

## Phase 1 : Fondations et Configuration
- [x] **Task 1.1 :** Créer la structure des répertoires (ex: `cmd/`, `internal/config/`, `internal/scheduler/`, `internal/probes/`).
- [x] **Task 1.2 :** Mettre en place la commande racine `cobra` (`stabsight run` a minima) avec les flags globaux (`--config`, `--log-level`, `--otlp-endpoint`, `--metrics-addr`).
- [x] **Task 1.3 :** Implémenter le chargement de `config.yaml` via `viper` avec validation des types (structures pour `targets`/sondes), en réservant cette partie du fichier au YAML uniquement (pas de binding flag/env dessus).
- [x] **Task 1.4 :** Configurer la surcharge par variables d'environnement sur les seuls réglages opérationnels (`viper.SetEnvPrefix("INTERNET_MONITOR")`, `viper.AutomaticEnv()`, `SetEnvKeyReplacer`) et écrire un test vérifiant l'ordre de priorité flag > env > fichier > défaut.
- [x] **Task 1.5 :** Mettre en place le logger structuré global (ex: `rs/zerolog`), initialisé à partir du niveau de log résolu par Viper (flag/env/YAML).

## Phase 2 : Le Cœur de l'Orchestrateur (Scheduler)
- [x] **Task 2.1 :** Définir les types structurés pour les Channels (`Command`, `CmdPause`, `CmdResume`).
- [x] **Task 2.2 :** Créer l'interface abstraite `ProbeWorker` que toutes les futures sondes devront respecter.
- [x] **Task 2.3 :** Coder le moteur du `Scheduler` capable de recenser les workers.
- [x] **Task 2.4 :** Implémenter et tester unitairement la logique de broadcast du `Pause`, la collecte des `Ack`, et le broadcast du `Resume` avec des workers "mock".

## Phase 3 : Instrumentation OpenTelemetry
- [x] **Task 3.1 :** Configurer le provider OTLP global au démarrage de l'application.
- [x] **Task 3.2 :** Instancier l'exporter Prometheus pour exposer le endpoint `/metrics`.
- [x] **Task 3.3 :** Instancier l'exporter OTLP (gRPC/HTTP) basé sur les variables d'environnement (standard OTel).
- [x] **Task 3.4 :** Créer un package utilitaire interne pour enregistrer facilement les métriques (enregistrement des Native Histograms).

## Phase 4 : Implémentation des Sondes "Légères"
- [x] **Task 4.1 :** Implémenter la sonde **ICMP** (utilisation de `golang.org/x/net/icmp` avec gestion correcte des privilèges `CAP_NET_RAW` ou UDP non-privilégié).
- [x] **Task 4.2 :** Implémenter la sonde **DNS** (mesure du temps de résolution).
- [ ] **Task 4.3 :** Implémenter la sonde **HTTP/TCP** (mesure TTFB, TLS handshake).
- [ ] **Task 4.4 :** Intégrer les calculs de Jitter spécifiques à ces sondes et injecter les résultats dans le provider OTLP.

## Phase 5 : Sondes "Lourdes" et Diagnostics
- [ ] **Task 5.1 :** Implémenter la sonde **Speedtest** (utilisation d'un client natif Go ou wrap léger de CLI externe si trop complexe, avec exécution via le mode "Exclusif" du Scheduler).
- [ ] **Task 5.2 :** Implémenter la sonde **MTR** (exécution séquentielle de requêtes avec TTL incrémental).
- [ ] **Task 5.3 :** Créer la logique d'analyse locale corrélant les données MTR et ICMP pour émettre les logs structurés de diagnostic.

## Phase 6 : Finalisation et Déploiement
- [ ] **Task 6.1 :** Rédiger le `Dockerfile` optimisé multi-stage.
- [ ] **Task 6.2 :** Rédiger le `README.md` avec un exemple de configuration et d'intégration (ex: configuration Grafana Alloy pour ingestion).
- [ ] **Task 6.3 :** Tests d'intégration réseau finaux.
