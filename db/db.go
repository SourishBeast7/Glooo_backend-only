package db

import (
	"errors"
	"log"
	"os"

	"github.com/SourishBeast7/Glooo/db/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Storage struct {
	db *gorm.DB
}

type Response map[string]any

func NewStorage() *Storage {
	dsn := os.Getenv("DB_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {

		log.Printf("%v", err)
		return nil
	}
	return &Storage{
		db: db,
	}
}

func (s *Storage) Init() error {
	if err := s.db.AutoMigrate(&models.User{}, &models.Chat{}, &models.Message{}, &models.FriendRequest{}); err != nil {
		return err
	}

	log.Println("✅ Database migrated")
	return nil
}

//User

func (s *Storage) CreateUser(user *models.User) error {
	u := new(models.User)
	if err := s.db.Where("email = ?", user.Email).First(u).Error; err == nil {
		return errors.New("user already exists")
	}
	pass, err := bcrypt.GenerateFromPassword([]byte(user.Password), 10)
	if err != nil {
		return err
	}
	user.Password = string(pass)
	if err := s.db.Model(&models.User{}).Create(user).Error; err != nil {
		return err
	}
	return nil
}

func (s *Storage) AuthenticateUser(email string, password string) (*models.User, error) {
	user := new(models.User)
	if err := s.db.Preload("Chats").Preload("Friends").Model(&models.User{}).Where("email = ?", email).Find(user).Error; err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Storage) FindUserByEmailGorm(email string) (*models.User, error) {
	u := new(models.User)
	if err := s.db.Where("email = ?", email).Find(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Storage) UpdateUser(user *models.UpdateUser) (bool, error) {
	allowedFields := map[string]bool{
		"Name":     true,
		"Email":    true,
		"Pfp":      true,
		"Password": true,
	}
	if !allowedFields[user.Field] {
		return false, errors.New("field update not allowed")
	}
	if err := s.db.Model(&models.User{}).Where("email = ?", user.Email).Update(user.Field, user.Value).Error; err != nil {
		return false, err
	}

	return true, nil
}

func (s *Storage) FindUserById(id uint) (*models.User, error) {
	user := new(models.User)
	if err := s.db.Table("users").Select("name,email,created_at,id").Where("id = ?", id).Find(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

//Friends

func (s *Storage) GetReceivedFriendRequest(uid uint) ([]*models.FriendRequest, error) {
	user := new(models.User)
	if err := s.db.Model(&models.User{}).Preload("ReceivedRequests").Where("id = ?", uid).First(user).Error; err != nil {
		return nil, err
	}
	return user.ReceivedRequests, nil
}

func (s *Storage) SendFriendRequest(userEmail string, friendEmail string) error {
	if userEmail == friendEmail {
		return errors.New("user cannot add themselves as a friend")
	}

	var user, friend models.User

	// Find user
	if err := s.db.Table("users").Where("email = ?", userEmail).First(&user).Error; err != nil {
		log.Printf("user not found: %v", err)
		return err
	}

	// Find friend
	if err := s.db.Table("users").Where("email = ?", friendEmail).First(&friend).Error; err != nil {
		log.Printf("friend not found: %v", err)
		return err
	}

	if err := s.db.Create(&models.FriendRequest{
		FromID: user.ID,
		ToID:   friend.ID,
		Status: "pending",
	}).Error; err != nil {
		return err
	}
	// // Add friend to user's Friends list
	// if err := s.db.Model(&user).Association("Friends").Append(&friend); err != nil {
	// 	log.Printf("failed to add friend: %v", err)
	// 	return err
	// }

	// // Optional: Add user to friend's Friends list for mutual friendship
	// if err := s.db.Model(&friend).Association("Friends").Append(&user); err != nil {
	// 	log.Printf("failed to add reverse friend: %v", err)
	// 	return err
	// }

	return nil
}

func (s *Storage) HandleFriendRequest(action string, email string) error {
	user := new(models.User)
	if err := s.db.Find(user, email).Error; err != nil {
		return err
	}
	req := new(models.FriendRequest)
	if err := s.db.Table("friend_requests").Where("to_id = ?", user.ID).Find(req).Error; err != nil {
		return err
	}
	if action == "decline" {
		if err := s.db.Delete(req).Error; err != nil {
			return err
		}
		log.Println("Request Declined")
		return nil
	}
	if action == "accept" {
		if req.Status == "pending" || req.Status == "accepted" || req.Status == "deleted" {
			return errors.New("Request is " + req.Status)
		}
		friend := new(models.User)
		if err := s.db.Model(friend).Association("Friends").Append(user); err != nil {
			return err
		}
		if err := s.db.Model(user).Association("Friends").Append(friend); err != nil {
			return err
		}
		return nil
	}
	return errors.New("action unrecognised")
}

func (s *Storage) GetFriends(user_id uint) ([]*models.User, error) {
	user := new(models.User)
	if err := s.db.Model(&models.User{}).Preload("Friends").Where("id = ?", user_id).First(user).Error; err != nil {
		return nil, err
	}
	return user.Friends, nil
}

//Chats

func (s *Storage) FindChatByChatID(id uint) (*models.Chat, error) {
	chat := new(models.Chat)
	if err := s.db.Model(&models.Chat{}).Preload("Users").Preload("Messages").Where("id = ?", id).First(chat).Error; err != nil {
		return nil, err
	}
	return chat, nil
}

func (s *Storage) CreateChat(name string, users ...*models.User) (*models.Chat, error) {
	chat := new(models.Chat)
	chat.Users = users
	if name == "" {
		if len(users) > 2 {
			chat.Name = "group"
		}
		chat.Name = "user"
	}
	if err := s.db.Create(chat).Error; err != nil {
		return nil, err
	}
	return chat, nil
}

func (s *Storage) GetChatsByUserId(user_id uint) ([]*models.Chat, error) {
	user := models.User{}
	if err := s.db.Preload("Chats").Where("id = ?", user_id).First(&user).Error; err != nil {
		return nil, err
	}
	return user.Chats, nil
}

// Messages

func (s *Storage) GetMessages(chat_id uint) ([]*models.Message, error) {
	messages := make([]*models.Message, 0)
	if err := s.db.Model(&models.Message{}).Where("chat_id = ?", chat_id).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *Storage) AddMessages(msg *models.Message) (Response, error) {

	chat := new(models.Chat)
	if err := s.db.
		Joins("JOIN user_chats uc1 ON uc1.chat_id = chats.id AND uc1.user_id = ?", msg.SenderID).
		Joins("JOIN user_chats uc2 ON uc2.chat_id = chats.id AND uc2.user_id = ?", msg.ReceiverID).
		Where("is_group = ?", false). // optional: only private chats
		Find(chat).Error; err != nil {
		return Response{
			"msg": "Chat does not exist",
		}, err
	}
	msg.ChatID = chat.ID
	if err := s.db.Model(&models.Message{}).Create(msg).Error; err != nil {
		return nil, err
	}
	return Response{
		"success": true,
	}, nil
}
