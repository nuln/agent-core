package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCardBuilder(t *testing.T) {
	b := NewCard().
		Title("Test Card", "blue").
		Markdown("Hello *world*").
		Divider().
		Buttons(PrimaryBtn("Click Me", "cmd:/test")).
		Note("Footnote")

	card := b.Build()

	assert.Equal(t, "Test Card", card.Header.Title)
	assert.Equal(t, "blue", card.Header.Color)
	assert.Len(t, card.Elements, 4)
}

func TestCardRendering(t *testing.T) {
	card := NewCard().
		Title("Title", "red").
		Markdown("Content").
		Buttons(DefaultBtn("Btn", "val")).
		Build()

	text := card.RenderText()
	assert.Contains(t, text, "**Title**")
	assert.Contains(t, text, "Content")
	assert.Contains(t, text, "[Btn]")

	assert.True(t, card.HasButtons())

	buttons := card.CollectButtons()
	assert.Len(t, buttons, 1)
	assert.Equal(t, "Btn", buttons[0][0].Text)
}
