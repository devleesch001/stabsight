# stabsight

Agent d'observabilité réseau léger et haute performance en Go, conçu pour mesurer avec précision la qualité, la stabilité et les performances d'une connexion Internet ou d'un réseau local/distant.

---

## Fonctionnalités clés

- **Sondes multi-protocoles légères et concurrentes :**
  - **ICMP Ping :** Mesure de latence RTT et taux de perte (support des sockets RAW privilégiés `CAP_NET_RAW` et repli automatique UDP non-privilégié `udp4`/`udp6`).
  - **DNS :** Mesure du temps de résolution avec types d'enregistrements configurables (`A`, `AAAA`, `CNAME`, `TXT`, etc.) et serveur de noms ciblé.
  - **HTTP/HTTPS :** Décomposition fine du temps de réponse (`httptrace`) avec mesure du TTFB (Time To First Byte), durée de handshake TLS, et validation du code de statut attendu.
  - **TCP Connect :** Mesure du temps d'établissement de connexion socket TCP.
- **Sondes lourdes et diagnostics avancés :**
  - **Speedtest (Bande passante) :** Mesure de débit Download et Upload natif.
  - **Orchestrateur anti-bufferbloat :** Suspension coordonnée et automatique de toutes les sondes légères pendant l'exécution d'un Speedtest (`Scheduler.ExecuteExclusive`), puis reprise immédiate sans interruption brutale des requêtes en cours.
  - **MTR (Traceroute) :** Diagnostic séquentiel à TTL incrémental (1..max_hops) capturant les sauts intermédiaires et le temps de transit.
  - **Diagnostic et corrélation locale :** Analyse automatique en cas de dégradation ou coupure avec qualification de l'incident (`local_network_issue`, `isp_gateway_loss`, `transit_route_loss`, `target_unreachable`) et émission de logs structurés JSON Zerolog.
- **Instrumentation OpenTelemetry & Native Histograms :**
  - Métriques OpenTelemetry standardisées avec agrégation en histogrammes exponentiels natifs (`ExponentialBucketHistogram` à haute résolution sans buckets statiques arbitraires).
  - Double mode d'export : Push OTLP (gRPC sur le port 4317 ou HTTP sur le port 4318) et Pull Prometheus (`/metrics` HTTP).

---

## Métriques OpenTelemetry exposées

Toutes les métriques respectent le préfixe conventionnel `internet_monitor_` :

| Métrique | Type | Unité | Description | Labels standards |
|---|---|---|---|---|
| `internet_monitor_rtt_seconds` | Float64Histogram (Native) | `s` | Latence aller-retour (RTT) | `target`, `probe`, `ip_version`, `status_code`, `record_type`, `hop`, `hop_ip` |
| `internet_monitor_jitter_seconds` | Float64Histogram (Native) | `s` | Variation instantanée de latence \|RTT_n - RTT_(n-1)\| (sans lissage) | `target`, `probe`, `ip_version`, `status_code`, `record_type`, `port` |
| `internet_monitor_packet_loss_ratio` | Float64Gauge | `1` | Ratio de perte de paquets (0.0 = 0%, 1.0 = 100%) | `target`, `probe`, `ip_version`, `status_code`, `record_type`, `port`, `hop`, `hop_ip` |
| `internet_monitor_speedtest_bits_per_second` | Float64Gauge | `bit/s` | Débit mesuré en bande passante (bits/s) | `target`, `probe="speedtest"`, `direction` ("download", "upload") |

---

## Configuration

La configuration s'effectue via un fichier YAML unique (ex: `config.yaml`). Les réglages opérationnels (chemins, adresses d'écoute, endpoints OTLP, niveaux de log) peuvent également être surchargés par variables d'environnement (`INTERNET_MONITOR_*` ou standards OTel `OTEL_EXPORTER_OTLP_*`).

### Exemple de configuration (`config.yaml`)

```yaml
metrics_addr: ":9090"
otlp_endpoint: "localhost:4317"
log_level: "info"

targets:
  - name: "google-dns"
    host: "8.8.8.8"
    ip_version: "ipv4"
    probes:
      icmp:
        interval: "1s"
        timeout: "1s"
        count: 1
      dns:
        interval: "5s"
        timeout: "2s"
        server: "8.8.8.8:53"
        record_type: "A"
      mtr:
        interval: "60s"
        timeout: "1s"
        max_hops: 30

  - name: "cloudflare-web"
    host: "1.1.1.1"
    ip_version: "ipv4"
    probes:
      tcp:
        interval: "5s"
        timeout: "2s"
        port: 443
      http:
        interval: "10s"
        timeout: "5s"
        url: "https://1.1.1.1"
        method: "GET"
        expected_code: 200

  - name: "local-isp-speed"
    host: "speedtest"
    probes:
      speedtest:
        interval: "30m"
        timeout: "60s"
```

---

## Intégration Grafana Alloy

Pour ingérer les métriques via **Grafana Alloy** (ou un OpenTelemetry Collector standard) :

### Option 1 : Réception push OTLP (gRPC / HTTP)

```alloy
// Configuration Alloy pour recevoir les métriques poussées par stabsight
otelcol.receiver.otlp "default" {
  grpc {
    endpoint = "0.0.0.0:4317"
  }
  http {
    endpoint = "0.0.0.0:4318"
  }

  output {
    metrics = [otelcol.exporter.prometheus.mimir.input]
  }
}

otelcol.exporter.prometheus "mimir" {
  forward_to = [prometheus.remote_write.default.receiver]
}

prometheus.remote_write "default" {
  endpoint {
    url = "http://mimir:9009/api/v1/push"
  }
}
```

### Option 2 : Scraping Prometheus (Pull `/metrics`)

```alloy
prometheus.scrape "stabsight" {
  targets = [
    {"__address__" = "localhost:9090"},
  ]
  metrics_path = "/metrics"
  scrape_interval = "5s"
  forward_to = [prometheus.remote_write.default.receiver]
}
```

---

## Utilisation

### Compilation & Exécution locale

```bash
# Compiler le binaire
go build -o bin/stabsight ./cmd

# Lancer la surveillance
./bin/stabsight run --config config.example.yaml --log-level debug

# Consulter la version
./bin/stabsight version
```

### Exécution avec Docker

```bash
# Construction de l'image
docker build -t stabsight:latest .

# Exécution du conteneur avec exposition du endpoint Prometheus
docker run -d \
  --name stabsight \
  -p 9090:9090 \
  -v $(pwd)/config.example.yaml:/etc/stabsight/config.yaml:ro \
  stabsight:latest
```

---

## Développement et Tests

```bash
# Vérification du code
go vet ./...

# Tests unitaires avec détection de data races et couverture
go test ./... -race -cover

# Linter officiel
golangci-lint run

# Validation de la compilation multi-architecture (GoReleaser)
goreleaser build --snapshot --clean
```
