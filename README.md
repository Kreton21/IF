# IF Festival - Billetterie en ligne

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Frontend   │────▶│  Go Backend  │────▶│  HelloAsso   │
│  (Client)    │◀────│   (API)      │◀────│   API v5     │
└─────────────┘     └──────┬───────┘     └──────────────┘
                           │
                    ┌──────┴───────┐
                    │              │
              ┌─────▼─────┐ ┌─────▼─────┐
              │ PostgreSQL │ │   Redis    │
              │  (données) │ │  (cache)   │
              └───────────┘ └───────────┘
```

## Flux d'achat

1. Le client choisit ses tickets sur le site
2. Le backend crée un **checkout intent** via HelloAsso API
3. Le client est redirigé vers la page de paiement HelloAsso
4. HelloAsso envoie un **webhook** pour confirmer le paiement
5. Le backend génère un **QR code** unique
6. Le QR code est envoyé par **email** au client

## Stack technique

| Composant | Technologie | Justification |
|-----------|-------------|---------------|
| Backend | Go + Chi | Performance, concurrence native |
| DB | PostgreSQL | ACID, transactions financières, concurrent R/W |
| Cache | Redis | Validation QR rapide, rate limiting, sessions |
| QR Code | go-qrcode | Génération côté serveur |
| Auth Admin | JWT | Stateless, scalable |
| Paiement | HelloAsso API v5 | Gestion paiement + billetterie |

## Lancement

```bash
# Démarrer les services
docker-compose up -d

# Lancer le backend
cd backend && go run cmd/server/main.go

# Le frontend est servi par le backend sur /
# L'admin est servi sur /admin/
```

## Variables d'environnement

Copier `.env.example` en `.env` et remplir les valeurs.

## API Endpoints

### Public
- `POST /api/v1/tickets/checkout` — Créer un checkout HelloAsso
- `GET  /api/v1/tickets/types` — Lister les types de tickets disponibles
- `GET  /api/v1/orders/:id/status` — Vérifier le statut d'une commande

### Webhooks
- `POST /api/v1/webhooks/helloasso` — Webhook HelloAsso (confirmation paiement)

### Admin (JWT requis)
- `POST   /api/v1/admin/login` — Connexion admin
- `GET    /api/v1/admin/stats` — Statistiques de ventes
- `GET    /api/v1/admin/orders` — Liste des commandes
- `POST   /api/v1/admin/validate-qr` — Valider un QR code à l'accueil
- `GET    /api/v1/admin/ticket-types` — Gérer les types de tickets
- `POST   /api/v1/admin/ticket-types` — Créer un type de ticket
