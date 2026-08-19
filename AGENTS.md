# AGENTS.md — Internet-Monitor

Agent d'observabilité réseau en Go. Ce fichier ne décrit **pas** le projet — il décrit comment toi, agent de code, dois te comporter dessus. La spec fonctionnelle et technique vit ailleurs, lis-la avant de coder.

## À lire avant toute tâche

- `specs/design.md` — architecture cible, découplage des composants, choix techniques.
- `specs/requirements.md` — besoin, portée, critères d'acceptation.
- `specs/tasks.md` — découpage en tâches, ordre d'exécution attendu.

Ne code jamais à partir de ta seule compréhension du projet ou de ce fichier : ces trois fichiers sont la source de vérité. En cas de conflit apparent entre ce que tu es en train de coder et `design.md`/`requirements.md`, arrête-toi et signale l'écart plutôt que de trancher toi-même.

## Build & Test (commandes de base)

```bash
go build ./...
go vet ./...
go test ./... -race -cover
golangci-lint run
goreleaser build --snapshot --clean
```

Lance systématiquement `-race` avant de considérer une tâche terminée : le projet est fortement concurrent, une régression de concurrence ne se voit pas sans ce flag.

`golangci-lint run` doit passer sans erreur avant de marquer une tâche comme terminée — pas seulement `go build`/`go vet`. Ne désactive jamais un linter dans `.golangci.yml` pour faire passer une tâche plus vite ; si une règle bloque légitimement, signale-le au lieu de la contourner.

Le build de release passe exclusivement par `goreleaser` (config dans `.goreleaser.yml`), jamais par un `go build` manuel avec des flags de cross-compilation à la main. Utilise `goreleaser build --snapshot --clean` pour valider localement qu'une modification ne casse pas le build multi-arch, avant de la considérer terminée si elle touche à la compilation, aux tags de build ou aux dépendances CGO.

## Comportement attendu sur ce projet

0. **Explique avant d'agir, jamais après.** Avant toute modification (code, fichier de spec, config, commande destructive), annonce en une ou deux phrases ce que tu vas faire et pourquoi, *puis* exécute. Ne fais jamais l'inverse (agir d'abord, résumer ensuite) — un résumé après coup n'est pas une explication, c'est un compte-rendu que l'utilisateur ne peut plus arrêter. Cette règle s'applique à chaque tâche, sans exception, quel que soit l'agent utilisé.

1. **Respecte l'ordre de `tasks.md`.** Ne code pas une sonde ou une couche d'export si une tâche antérieure listée (typiquement le scheduler) n'est pas terminée et testée. Si tu penses qu'un autre ordre serait plus efficace, demande avant de dévier.
2. **N'assouplis jamais un critère d'acceptation pour faire passer un test.** Si un test échoue à cause d'une contrainte de `requirements.md` (ex: aucune requête réseau interrompue pendant un `Pause`), corrige le code, ne réécris pas le test ni la contrainte.
3. **Ne réintroduis pas ce que `design.md` interdit explicitement**, même si ça semble être un raccourci pratique localement (ex: un flag de compatibilité, un import direct d'un exporteur dans le code des sondes). Si `design.md` ne tranche pas un point, demande plutôt que de décider seul et de le documenter après coup.
4. **Une tâche = une unité testée et un commit.** Ne marque pas une tâche de `tasks.md` comme terminée sans les tests correspondants qui passent avec `-race` et `golangci-lint`. Effectue systématiquement un commit git clair et atomique à la fin de chaque tâche validée. Ne fais pas de commit "gros paquet" qui mélange plusieurs tâches non liées.
5. **Signale les ambiguïtés au lieu de les combler silencieusement.** Si `requirements.md` ou `design.md` est muet sur un point (ex: comportement en cas de timeout DNS pendant un Speedtest), pose la question plutôt que d'inventer un comportement par défaut non documenté.
6. **Ne modifie pas les specs pour qu'elles correspondent au code.** Si tu identifies une incohérence ou une amélioration possible dans `design.md`/`requirements.md`/`tasks.md`, propose-la explicitement à l'utilisateur ; ne les édite pas de ta propre initiative.

## Conventions de code (rappel court — le détail est dans design.md)

- Logs structurés : Zerolog uniquement.
- Métriques : OpenTelemetry/OTLP uniquement, jamais le client Prometheus directement.
- Config : un seul fichier YAML en entrée (`config.example.yaml`), pas de flags CLI pour définir cibles/sondes.
- Binaire statique cross-compilable AMD64/ARM64, Dockerfile minimal (Alpine/distroless).
- Release et cross-compilation gérées par `goreleaser` — ne pas dupliquer cette logique dans un script maison ou un Makefile parallèle.