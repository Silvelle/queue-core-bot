package handler

import (
	"errors"
	"fmt"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/service"
)

const (
	helpText = `Я веду очереди на защиту в этом чате.

Обозначения:
<…> — обязательно
<*…> — можно не указывать
«или» — подойдёт любой из вариантов

/new <название> — создать очередь: /new Практика 4
/swap <номер> — поменяться с тем, кто на этом месте: /swap 5
/swap <кто> <с кем> — поменять местами двух человек: /swap 1 3
/add <ID или @имя> <*место> — добавить человека, без места — в конец: /add @username 3
/place <номер, ID или @имя> <*место> — переставить человека, без места — в конец: /place 7 3
/delete <номер, ID или @имя> — убрать человека из очереди: /delete 4
/close <название> — закрыть очередь
/queues — список открытых очередей
/show <название> — показать доску очереди внизу чата

Встать в очередь и выйти — кнопками под доской.
/add можно отправить ответом на сообщение человека, тогда имя не нужно.
Если очередей несколько, отправьте команду ответом на доску нужной.`

	privateChatText = "Очереди работают в групповых чатах. Добавьте меня в группу и отправьте там /new <название>."
	newUsageText    = "Укажите название, например /new Практика 4"
	badButtonText   = "Эта кнопка больше не работает."
	genericErrText  = "Что-то пошло не так. Попробуйте ещё раз."

	swapUsageText   = "Укажите, с кем поменяться: /swap 5. Или кого с кем: /swap 1 3 — по номерам или ID."
	deleteUsageText = "Укажите, кого убрать из очереди: /delete 4 — по номеру или ID."
	placeUsageText  = "Укажите, кого и куда переставить: /place 7 3 — человека с №7 на №3, /place 7 — в конец. Вместо номера можно указать ID или @имя."
	addUsageText    = "Укажите, кого добавить: /add @имя, /add <ID> или ответьте командой /add на сообщение этого человека. Место можно указать после: /add @имя 3, иначе — в конец."
	closeUsageText  = "Укажите очередь, например /close Практика 4, или отправьте /close ответом на её доску."
	noQueuesText    = "Здесь нет открытых очередей. Создайте: /new <название>"
	whichQueueText  = "Открыто несколько очередей. Отправьте команду ответом на доску нужной."
	noSuchQueueText = "Здесь нет открытой очереди с таким названием."
	showUsageText   = "Укажите очередь, например /show Практика 4, или отправьте /show ответом на её доску."

	leftText = "Вы вышли из очереди."
)

// errorTexts is what a person sees for each rule the service enforces.
var errorTexts = map[error]string{
	model.ErrNotFound:         "Этой очереди больше нет.",
	model.ErrQueueClosed:      "Очередь закрыта.",
	model.ErrAlreadyJoined:    "Вы уже в очереди.",
	model.ErrNotInQueue:       "Вас нет в этой очереди. Сначала нажмите «+ Записаться».",
	model.ErrQueueExists:      "Очередь с таким названием уже открыта.",
	model.ErrInvalidName:      "Название должно быть от 1 до 64 символов.",
	model.ErrInvalidPosition:  "На этом месте никого нет.",
	model.ErrSelfSwap:         "Нельзя поменяться местами с самим собой.",
	model.ErrTargetNotInQueue: "Этого человека нет в очереди. Добавить его можно командой /add.",
	model.ErrTargetInQueue:    "Этот человек уже в очереди. Переставить его можно командой /place.",
	model.ErrAlreadyThere:     "Этот человек уже стоит на этом месте.",
	model.ErrUnknownUser:      "Человека с таким ID нет в этом чате.",
}

// errorText turns a service error into a short message. known is false for
// errors that aren't the person's fault, like a broken database: they get a
// generic message, and their details belong in the log, not the chat.
func errorText(err error) (text string, known bool) {
	for target, text := range errorTexts {
		if errors.Is(err, target) {
			return text, true
		}
	}
	return genericErrText, false
}

func joinedText(pos int) string {
	return fmt.Sprintf("Вы записались. Ваше место: №%d.", pos)
}
func toEndText(pos int) string {
	return fmt.Sprintf("Вы перешли в конец. Ваше место: №%d.", pos)
}

func whereText(pos, total int) string {
	if pos == 0 {
		return "Вас нет в этой очереди."
	}
	if pos == 1 {
		return fmt.Sprintf("Вы №1 из %d. Сейчас ваша очередь!", total)
	}
	return fmt.Sprintf("Вы №%d из %d. Перед вами: %d.", pos, total, pos-1)
}

// swappedText is the public line posted after a swap, so everyone can see
// who moved whom. Positions are the ones before the swap.
func swappedText(queue, user, target string, from, to int) string {
	return fmt.Sprintf("%s: %s №%d ⇄ %s №%d", queue, user, from, target, to)
}

// placedText is the public line posted after /place and /add: they move
// other people, so everyone should see it.
func placedText(queue, user string, p service.Placed) string {
	if p.Added() {
		return fmt.Sprintf("%s добавлен в очередь под номером %d", user, p.To)
	}
	return fmt.Sprintf("%s: %s №%d → №%d", queue, user, p.From, p.To)
}

// unknownMentionText explains a @username the bot can't match to anyone.
func unknownMentionText(name string) string {
	return fmt.Sprintf("Я пока не знаю %s: пусть он(а) хоть раз нажмёт кнопку бота. Или ответьте командой на его сообщение, или укажите его ID.", name)
}

// removedText is the public line posted after /delete.
func removedText(queue, user string, from int) string {
	return fmt.Sprintf("%s: %s убран(а) из очереди, место №%d освободилось", queue, user, from)
}

func closedText(queue string) string {
	return fmt.Sprintf("Очередь «%s» закрыта.", queue)
}

// pickErrorText explains why pickQueue couldn't find the queue a command
// is about. usage is shown when the command needs more information.
func pickErrorText(err error, usage string) string {
	switch {
	case errors.Is(err, errNoOpenQueues):
		return noQueuesText
	case errors.Is(err, errNoSuchName):
		return noSuchQueueText
	case errors.Is(err, errWhichQueue):
		if usage != "" {
			return usage
		}
		return whichQueueText
	}
	return genericErrText
}
