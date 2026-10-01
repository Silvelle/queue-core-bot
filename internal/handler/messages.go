package handler

import (
	"errors"
	"fmt"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

const (
	helpText = `Я веду очереди на защиту в этом чате.

/new <название> — создать очередь, например /new Практика 4
/swap <номер> — поменяться местами с тем, кто стоит на этом месте
/close <название> — закрыть очередь
/queues — список открытых очередей
/show <название> — показать доску очереди внизу чата

Кнопки под очередью действуют только на того, кто их нажал.
«↩ Сброс» возвращает на прежнее место, если вы нажали «✓ Сдано» по ошибке.

Если открыто несколько очередей, отправляйте /swap и /close ответом на доску нужной.`

	privateChatText = "Очереди работают в групповых чатах. Добавьте меня в группу и отправьте там /new <название>."
	newUsageText    = "Укажите название, например /new Практика 4"
	badButtonText   = "Эта кнопка больше не работает."
	genericErrText  = "Что-то пошло не так. Попробуйте ещё раз."

	swapUsageText   = "Укажите номер места, например /swap 5"
	closeUsageText  = "Укажите очередь, например /close Практика 4, или отправьте /close ответом на её доску."
	noQueuesText    = "Здесь нет открытых очередей. Создайте: /new <название>"
	whichQueueText  = "Открыто несколько очередей. Отправьте команду ответом на доску нужной."
	noSuchQueueText = "Здесь нет открытой очереди с таким названием."
	showUsageText   = "Укажите очередь, например /show Практика 4, или отправьте /show ответом на её доску."

	leftText = "Вы вышли из очереди."
	doneText = "Отмечено: сдано. Если по ошибке, нажмите «↩ Сброс»."
)

// errorTexts is what a person sees for each rule the service enforces.
var errorTexts = map[error]string{
	model.ErrNotFound:         "Этой очереди больше нет.",
	model.ErrQueueClosed:      "Очередь закрыта.",
	model.ErrAlreadyJoined:    "Вы уже в очереди.",
	model.ErrAlreadyDone:      "Вы уже сдали в этой очереди. Если по ошибке, нажмите «↩ Сброс».",
	model.ErrNotInQueue:       "Вас нет в этой очереди. Сначала нажмите «+ Записаться».",
	model.ErrQueueExists:      "Очередь с таким названием уже открыта.",
	model.ErrInvalidName:      "Название должно быть от 1 до 64 символов.",
	model.ErrInvalidPosition:  "На этом месте никого нет.",
	model.ErrSelfSwap:         "Нельзя поменяться местами с самим собой.",
	model.ErrTargetNotInQueue: "Этого человека нет среди ожидающих.",
	model.ErrNotDone:          "Вы ещё не отмечали «Сдано».",
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
func undoText(pos int) string {
	return fmt.Sprintf("Отметка снята. Ваше место: №%d.", pos)
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
