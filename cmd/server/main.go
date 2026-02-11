package main

import (
	"context"
	"log"
	"net/http"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/cases"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/config"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/db"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/events"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/gsi"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/inventory"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/lottery"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/stream"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/telegram"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/wallet"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	authService := auth.NewService(pool, cfg.JWTSecret, cfg.BaseURL)
	walletService := wallet.NewService(pool)
	walletHandler := wallet.NewHandler(walletService)
	inventoryService := inventory.NewService(pool, walletService)
	inventoryHandler := inventory.NewHandler(inventoryService)
	eventsService := events.NewService(pool)
	eventsHandler := events.NewHandler(eventsService)
	lotteryService := lottery.NewService(pool, walletService)
	lotteryHandler := lottery.NewHandler(lotteryService)
	botClient, err := telegram.NewBotClient(cfg.TelegramBotToken, cfg.BaseURL)
	if err != nil {
		log.Fatalf("telegram bot startup failed: %v", err)
	}
	streamService := stream.NewService(pool, lotteryService, inventoryService, botClient, cfg.BaseURL, cfg.TelegramBotUsername)
	streamHandler := stream.NewHandler(streamService)
	authHandler := auth.NewHandler(authService, streamService)
	casesService := cases.NewService(pool, walletService, lotteryService, inventoryService)
	casesHandler := cases.NewHandler(casesService)
	gsiHandler := gsi.NewHandler(eventsService, lotteryService, streamService, pool)
	telegramHandler := telegram.NewHandler(authService, cfg.TelegramBotToken)

	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Post("/api/telegram/webhook", botClient.HandleWebhook)
	r.Get("/invite/{inviteCode}", streamHandler.InviteLanding)

	r.Route("/api", func(api chi.Router) {
		api.Post("/auth/register", authHandler.Register)
		api.Post("/auth/login", authHandler.Login)
		api.Post("/auth/logout", authHandler.Logout)
		api.Post("/telegram/auth", telegramHandler.Login)
		api.Get("/auth/steam/login", authHandler.SteamLogin)
		api.Get("/auth/steam/callback", authHandler.SteamCallback)
		api.Get("/events", eventsHandler.ListRecent)
		api.Post("/gsi", gsiHandler.Ingest)
		api.Get("/lottery/rounds", lotteryHandler.ListRounds)
		api.Get("/cases", casesHandler.List)
		api.Get("/streams/events/presets", streamHandler.ListEventPresets)

		api.Group(func(authed chi.Router) {
			authed.Use(authService.AuthMiddleware)
			authed.Get("/auth/me", authHandler.Me)
			authed.Post("/auth/become-streamer", authHandler.BecomeStreamer)
			authed.Post("/events", eventsHandler.Create)
			authed.Get("/events/me", eventsHandler.ListMine)
			authed.Post("/lottery/join", lotteryHandler.Join)
			authed.Get("/wallet/me", walletHandler.GetMyWallet)
			authed.Post("/wallet/topup", walletHandler.TopUp)
			authed.Get("/wallet/transactions", walletHandler.ListMyTransactions)
			authed.Get("/inventory/me", inventoryHandler.ListMine)
			authed.Post("/inventory/open/{itemID}", inventoryHandler.OpenCase)
			authed.Post("/inventory/sell/{itemID}", inventoryHandler.SellItem)
			authed.Post("/cases/{caseID}/contribute", casesHandler.Contribute)
			authed.Get("/crowdfunding/invite/{inviteCode}", casesHandler.CampaignByInvite)
			authed.Post("/streams/join/{inviteCode}", streamHandler.JoinInviteAuthenticated)

			authed.Group(func(streamer chi.Router) {
				streamer.Use(authService.RequireRoles(auth.RoleStreamer, auth.RoleAdmin))
				streamer.Post("/cases", casesHandler.Create)
				streamer.Put("/cases/{caseID}", casesHandler.Update)
				streamer.Delete("/cases/{caseID}", casesHandler.Delete)
				streamer.Post("/streams/start", streamHandler.Start)
				streamer.Get("/streams/me/active", streamHandler.ActiveMine)
				streamer.Post("/streams/{sessionID}/end", streamHandler.End)
				streamer.Get("/streams/{sessionID}/participants", streamHandler.ListParticipants)
				streamer.Post("/streams/{sessionID}/giveaways", streamHandler.AddGiveawayRule)
				streamer.Get("/streams/{sessionID}/giveaways", streamHandler.ListGiveawayRules)
				streamer.Put("/streams/{sessionID}/giveaways/{ruleID}", streamHandler.UpdateGiveawayRule)
				streamer.Delete("/streams/{sessionID}/giveaways/{ruleID}", streamHandler.DeleteGiveawayRule)
				streamer.Post("/gsi/fake", gsiHandler.GenerateFake)
			})

			authed.Group(func(admin chi.Router) {
				admin.Use(authService.RequireRoles(auth.RoleAdmin))
				admin.Put("/admin/users/{userID}/role", authHandler.SetUserRole)
			})
		})
	})

	fileServer := http.FileServer(http.Dir(cfg.FrontendPath))
	r.Handle("/*", fileServer)

	log.Printf("CS2 LiveDrop server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
