package db

import (
	"errors"
	"fmt"
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

func (s *Storage) FindUsersUsingSubstring(id uint, str string) ([]*models.User, error) {
	users := make([]*models.User, 0)
	user := new(models.User)
	if err := s.db.Model(&models.User{}).Where("id = ?", id).First(user).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("email LIKE ?", "%"+str+"%").Find(&users).Error; err != nil {
		return nil, err
	}
	for i, v := range users {
		if v.Email == user.Email {
			return append(users[:i], users[i+1:]...), nil
		}
	}
	return nil, errors.New("an error occurred")
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
	if err := s.db.Model(&models.User{}).Preload("ReceivedRequests").Preload("SentRequests").Preload("Friends").Where("id = ?", id).Find(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

//Friends

func (s *Storage) GetReceivedFriendRequest(uid uint) ([]*models.FriendRequest, error) {
	user := new(models.User)
	if err := s.db.Model(&models.User{}).Preload("ReceivedRequests.From").Preload("ReceivedRequests.To").Where("id = ?", uid).First(user).Error; err != nil {
		return nil, err
	}
	return user.ReceivedRequests, nil
}

func (s *Storage) CheckRequestExists(uid uint, friendEmail string) bool {
	friend := new(models.User)
	if err := s.db.Model(&models.User{}).Where("email = ?", friendEmail).First(friend).Error; err != nil {
		return false
	}
	if err := s.db.Model(&models.FriendRequest{}).Where("from_id = ? and to_id = ?", uid, friend.ID).First(nil).Error; err != nil {
		return false
	}
	return true
}

func (s *Storage) SendFriendRequest(userid uint, friendEmail string) error {
	if s.CheckRequestExists(userid, friendEmail) {
		return fmt.Errorf("user %s has already sent you a friend request", friendEmail)
	}
	var user, friend models.User

	// Find user
	if err := s.db.Model(&models.User{}).Where("id = ?", userid).First(&user).Error; err != nil {
		log.Printf("user not found: %v", err)
		return err
	}

	// Find friend
	if err := s.db.Model(&models.User{}).Where("email = ?", friendEmail).First(&friend).Error; err != nil {
		log.Printf("friend not found: %v", err)
		return err
	}
	if userid == friend.ID {
		return errors.New("user cannot add themselves as friend")
	}
	if err := s.db.Create(&models.FriendRequest{
		From:   &user,
		FromID: user.ID,
		ToID:   friend.ID,
		Status: "pending",
		To:     &friend,
	}).Error; err != nil {
		return err
	}
	return nil
}

func (s *Storage) HandleFriendRequest(req *models.HandleRequest) error {
	tx := s.db.Begin()
	newReq := new(models.FriendRequest)
	if err := tx.Model(&models.FriendRequest{}).Preload("From").Preload("To").Where("to_id = ? and from_id = ?", req.ToID, req.FromID).First(newReq).Error; err != nil {
		return err
	}
	if req.Action == "decline" {
		if err := tx.Model(&models.FriendRequest{}).Delete(newReq).Error; err != nil {
			tx.Rollback()
			return err
		}
		log.Println("Request Declined")
		tx.Commit()
		return nil
	}
	if req.Action == "accept" {
		if newReq.Status != "pending" {
			return errors.New("Request is " + newReq.Status)
		}

		user, friend := newReq.From, newReq.To
		if err := tx.Model(friend).Association("Friends").Append(user); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Model(user).Association("Friends").Append(friend); err != nil {
			tx.Rollback()
			return err
		}
		tx.Commit()
		// Creating chat for friends
		if err := s.CreateChat("", user, friend); err != nil {
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

func (s *Storage) CreateChat(name string, users ...*models.User) error {
	chat := new(models.Chat)
	chat.Users = users
	if name == "" {
		if len(users) > 2 {
			chat.Name = "group"
		}
		chat.Name = "user"
	}
	if err := s.db.Create(chat).Error; err != nil {
		return err
	}
	return nil
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
