package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/SourishBeast7/Glooo/db"
	"github.com/SourishBeast7/Glooo/db/models"
	"github.com/SourishBeast7/Glooo/http-server/hub"
	m "github.com/SourishBeast7/Glooo/http-server/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

type Server struct {
	listenAddr string
	store      *db.Storage
	hub        *hub.Hub
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

type Response map[string]any

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // ✅ Allow all origins temporarily
	},
}

func NewServer(addr string) *Server {
	return &Server{
		listenAddr: addr,
		store:      db.NewStorage(),
		hub:        hub.NewHub(),
	}
}

func StringToUint(s string) uint {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Println("Invalid String")
		return 0
	}
	return uint(i)
}

func makeHttpHandler(f handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
			log.Printf("%v", err)
		}
	}
}

func WriteJson(w http.ResponseWriter, status int, v map[string]any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

func uploadFilesToCdn(_ io.Reader, _ string, _ string) (string, error) {
	return "", nil
}

func GenerateJWT(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"email":     user.Email,
		"name":      user.Name,
		"pfp":       user.Pfp,
		"createdAt": user.CreatedAt,
	}
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

func (s *Server) HandleRoutes() {
	router := mux.NewRouter()
	s.store.Init()
	go s.hub.Run(s.store)
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http:localhost:5173/*"},
		AllowCredentials: true,
		Debug:            true,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization"},
	})

	handler := c.Handler(router)
	s.handleAuthRoutes(router.PathPrefix("/auth").Subrouter())
	s.handleChatRoutes(router.PathPrefix("/chat").Subrouter())
	s.handleApiRoutes(router.PathPrefix("/api").Subrouter())
	s.handleFriendsRoute(router.PathPrefix("/user").Subrouter())
	s.handleTestingRoutes(router.PathPrefix("/test").Subrouter())

	router.HandleFunc("/", makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		return WriteJson(w, http.StatusOK, Response{
			"message": "Welcome",
		})
	}))
	log.Printf("🚀 Server started on http://localhost%s", s.listenAddr)
	http.ListenAndServe(s.listenAddr, handler)
}

func (s *Server) handleAuthRoutes(router *mux.Router) {

	router.HandleFunc("/signup", makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		user := new(models.User)
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			return err
		}
		user.Name = r.FormValue("name")
		user.Email = r.FormValue("email")
		user.Password = r.FormValue("password")
		file, header, err := r.FormFile("pfp")
		if err != nil {
			return err
		}
		defer file.Close()
		pfp, err := uploadFilesToCdn(file, user.Email, header.Filename)
		if err != nil {
			return err
		}
		user.Pfp = pfp
		err = s.store.CreateUser(user)
		if err != nil {
			WriteJson(w, http.StatusNotAcceptable, Response{
				"success": "false",
			})
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"success": "true",
			"message": "User created Successfully",
		})
	})).Methods(http.MethodPost)

	router.HandleFunc("/login", makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		u := new(models.LoginUser)
		if err := json.NewDecoder(r.Body).Decode(u); err != nil {
			return WriteJson(w, http.StatusNotAcceptable, Response{
				"message": u,
			})
		}
		user, err := s.store.AuthenticateUser(u.Email, u.Password)
		if err != nil {
			return err
		}
		token, err := GenerateJWT(user)
		if err != nil {
			WriteJson(w, http.StatusNotAcceptable, Response{
				"success": false,
			})
			return err
		}
		var secMode bool
		var siteMode http.SameSite
		env := os.Getenv("ENVIRONMENT")
		if env == "dev" {
			secMode = false
			siteMode = http.SameSiteLaxMode
		} else {
			secMode = true
			siteMode = http.SameSiteNoneMode
		}
		id := strconv.Itoa(int(user.ID))
		finalToken := fmt.Sprintf("Bearer %s", token)
		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    finalToken,
			HttpOnly: true,
			Path:     "/",
			SameSite: siteMode,
			Secure:   secMode, // Set to true in production with HTTPS
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "id",
			Value:    id,
			HttpOnly: true,
			Path:     "/",
			SameSite: siteMode,
			Secure:   secMode, // Set to true in production with HTTPS
		})
		return WriteJson(w, http.StatusOK, Response{
			"success": true,
		})
	})).Methods(http.MethodPost)
}

// Get Friends , Messages And Chats

func (s *Server) handleApiRoutes(router *mux.Router) {

	router.HandleFunc("/search-user/{email}", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		email := mux.Vars(r)["email"]
		cookie, err := r.Cookie("id")
		id := StringToUint(cookie.Value)
		users, err := s.store.FindUsersUsingSubstring(id, email)
		if err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"users": users,
		})
	})))

	router.HandleFunc("/getfriends", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		cookie, err := r.Cookie("id")
		if err != nil {
			return err
		}
		user_id := StringToUint(cookie.Value)
		friends, err := s.store.GetFriends(user_id)
		if err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"friends": friends,
		})
	})))

	router.HandleFunc("/getchats", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		cookie, err := r.Cookie("id")
		if err != nil {
			WriteJson(w, http.StatusNotAcceptable, Response{
				"err": err,
			})
			return err
		}
		uid := StringToUint(cookie.Value)
		if uid == 0 {
			return errors.New("invalid user id")
		}
		chats, err := s.store.GetChatsByUserId(uid)
		if err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"chats": chats,
		})
	}))).Methods(http.MethodGet)

	router.HandleFunc("/getmessages", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		chat_id := r.URL.Query().Get("chat_id")
		id := StringToUint(chat_id)
		if id == 0 {
			return errors.New("invalid request")
		}
		messages, err := s.store.GetMessages(id)
		if err != nil {
			WriteJson(w, http.StatusNotAcceptable, Response{
				"success": false,
			})
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"success":  true,
			"messages": messages,
		})
	}))).Methods(http.MethodGet)

}

//Find Friends and Friend Requests Route

func (s *Server) handleFriendsRoute(router *mux.Router) {
	router.HandleFunc("/friend_requests", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		cookie, err := r.Cookie("id")
		if err != nil {
			return err
		}
		uid := StringToUint(cookie.Value)
		friendRequests, err := s.store.GetReceivedFriendRequest(uid)
		if err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"requests": friendRequests,
		})
	}))).Methods(http.MethodGet)

	router.HandleFunc("/send_request/{email}", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		cookie, err := r.Cookie("id")
		if err != nil {
			return err
		}
		email := mux.Vars(r)["email"]
		id := StringToUint(cookie.Value)
		if err := s.store.SendFriendRequest(id, email); err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"success": true,
		})
	})))

	router.HandleFunc("/handle_request", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		req := new(models.HandleRequest)
		if err := json.NewDecoder(r.Body).Decode(req); err != nil {
			return err
		}
		if err := s.store.HandleFriendRequest(req); err != nil {
			return err
		}
		return WriteJson(w, http.StatusOK, Response{
			"success": "",
		})
	}))).Methods(http.MethodPost)
}

//Find Friends and Friend Requests Route - end

//WebSocket - Websocket routes

func (s *Server) handleChatRoutes(router *mux.Router) {
	// Websocket Route
	router.HandleFunc("/", m.AuthMiddleWare(makeHttpHandler(s.wsConnHandler))).Methods(http.MethodGet)
}

func (s *Server) wsConnHandler(w http.ResponseWriter, r *http.Request) error {
	log.Println("➡️ Incoming WebSocket request...")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ WebSocket upgrade failed:", err)
		return err
	}

	log.Println("✅ WebSocket connection upgraded")
	cookie, err := r.Cookie("id")
	if err != nil {
		return err
	}
	client := s.hub.NewClient(StringToUint(cookie.Value), conn)
	s.hub.Register <- client
	go s.hub.Readloop(client)
	return nil
}

//Testing Routes Start

func (s *Server) handleTestingRoutes(router *mux.Router) {
	// router.HandleFunc("/create", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
	// 	var data map[string]string
	// 	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
	// 		return err
	// 	}
	// 	cookie, err := r.Cookie("id")
	// 	if err != nil {
	// 		return err
	// 	}
	// 	user1, err := s.store.FindUserById(StringToUint(cookie.Value))
	// 	if err != nil {
	// 		return err
	// 	}
	// 	user2, err := s.store.FindUserByEmailGorm(data["email"])
	// 	if err != nil {
	// 		return err
	// 	}
	// 	s.store.CreateChat("", user1, user2)
	// 	return WriteJson(w, http.StatusOK, Response{
	// 		"msg": "Chat Created",
	// 	})
	// }))).Methods(http.MethodPost)
	router.HandleFunc("/t1", m.AuthMiddleWare(makeHttpHandler(func(w http.ResponseWriter, r *http.Request) error {
		return WriteJson(w, http.StatusOK, Response{
			"message": "Destination Reached",
		})
	})))
}

// Testing routes End
