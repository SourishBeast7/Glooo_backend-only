package models

import (
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Name             string           `gorm:"not null" json:"name"`
	Email            string           `gorm:"uniqueIndex;not null" json:"email"`
	Pfp              string           `gorm:"default:'https://images.unsplash.com/photo-1618979251882-0b40ef3617f0?q=80&w=687&auto=format&fit=crop&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxwaG90by1wYWdlfHx8fGVufDB8fHx8fA%3D%3D'" json:"pfp"`
	Password         string           `gorm:"not null" json:"-"`
	Friends          []*User          `gorm:"many2many:user_friends;joinForeignKey:UserID;joinReferences:FriendID" json:"friends"`
	Chats            []*Chat          `gorm:"many2many:user_chats" json:"chats"`
	SentRequests     []*FriendRequest `gorm:"foreignKey:FromID;constraint:OnDelete:CASCADE" json:"sent_requests"`
	ReceivedRequests []*FriendRequest `gorm:"foreignKey:ToID;constraint:OnDelete:CASCADE" json:"received_requests"`
	SentMessages     []*Message       `gorm:"foreignKey:SenderID;constraint:OnDelete:CASCADE"`
	ReceivedMessages []*Message       `gorm:"foreignKey:ReceiverID;constraint:OnDelete:CASCADE"`
}

type Chat struct {
	gorm.Model
	Name     string     `gorm:"not null" json:"name"`
	IsGroup  bool       `gorm:"default:false" json:"is_group"`
	Users    []*User    `gorm:"many2many:user_chats" json:"users"`
	Messages []*Message `gorm:"foreignKey:ChatID;constraint:OnDelete:CASCADE" json:"messages"`
}

type Message struct {
	gorm.Model
	Content    string `gorm:"type:text;not null" json:"content"`
	ChatID     uint   `gorm:"index;not null" json:"chat_id"`
	Chat       *Chat  `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	SenderID   uint   `gorm:"index;not null" json:"sender_id"`
	Sender     *User
	ReceiverID uint `gorm:"index;not null" json:"receiver_id"`
	Receiver   *User
}

type FriendRequest struct {
	gorm.Model
	FromID uint   `gorm:"index;not null" json:"from_id"`
	From   *User  `gorm:"foreignKey:FromID;constraint:OnDelete:CASCADE" json:"from"`
	ToID   uint   `gorm:"index;not null" json:"to_id"`
	To     *User  `gorm:"foreignKey:ToID;constraint:OnDelete:CASCADE" json:"to"`
	Status string `gorm:"type:enum('pending','accepted','rejected');default:'pending'" json:"status"`
}

type UpdateUser struct {
	Email string `json:"email"`
	Field string `json:"field"`
	Value string `json:"value"`
}

type LoginUser struct {
	Email    string
	Password string
}
