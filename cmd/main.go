package main

import (
	"log"
	"net/http"

	"cinema_queue/internal/adapters/redis"
	"cinema_queue/internal/booking"
	"cinema_queue/internal/utils"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /movies", listMovies)

	mux.Handle("GET /", http.FileServer(http.Dir("static")))

	store := booking.NewRedisStore(redis.NewClient("localhost:6379"))
	svc := booking.NewService(store)

	bookingHandler := booking.NewHandler(svc)

	mux.HandleFunc("GET /movies/{movieID}/seats", bookingHandler.ListSeats)
	mux.HandleFunc("POST /movies/{movieID}/seats/{seatID}/hold", bookingHandler.HoldSeat)

	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", bookingHandler.ConfirmSession)
	mux.HandleFunc("DELETE /sessions/{sessionID}", bookingHandler.ReleaseSession)

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

var movies = []movieResponse{
	{ID: "kolobok", Title: "Последний богатырь. Колобок (2026)", Rows: 5, SeatsPerRow: 5},
	{ID: "dune", Title: "Дюна: Часть вторая", Rows: 8, SeatsPerRow: 10},
	{ID: "chelovek-pauk", Title: "Человек-паук: Новый день", Rows: 10, SeatsPerRow: 15},
	{ID: "odisseya", Title: "Одиссея", Rows: 10, SeatsPerRow: 15},
}

func listMovies(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, movies)
}

type movieResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Rows        int    `json:"rows"`
	SeatsPerRow int    `json:"seats_per_row"`
}
