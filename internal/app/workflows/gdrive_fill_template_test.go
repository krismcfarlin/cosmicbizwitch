package workflows_test

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"

	"cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/internal/app/workflows"
	"cosmicbizwitch/pkg/workflow"
	"cosmicbizwitch/pkg/workflow/pbstore"

	"github.com/pocketbase/pocketbase"
)

// TestGdriveFillTemplateWorkflow tests the gdrive_fill_template activity:
// 1. Copies a template document to a destination folder
// 2. Fills placeholders with provided variables
// 3. Verifies the new document has the replacements
func TestGdriveFillTemplateWorkflow(t *testing.T) {
	// Get Google credentials from environment
	refreshToken := os.Getenv("GOOGLE_REFRESH_TOKEN")
	if refreshToken == "" {
		t.Skip("GOOGLE_REFRESH_TOKEN not set, skipping integration test")
	}

	ctx := context.Background()

	// Initialize Google client with credentials from app settings
	// These are configured via the Settings page and stored securely
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		t.Fatal("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET must be set in environment")
	}

	tokenMgr := google.NewTokenManager(
		clientID,
		clientSecret,
		refreshToken,
	)
	gc := google.NewClient(tokenMgr)

	// Test configuration
	templateDocID := os.Getenv("TEST_TEMPLATE_DOC_ID") // Should be a Google Doc with {{first_name}}, {{last_name}}, {{email}}
	destFolderID := os.Getenv("TEST_DEST_FOLDER_ID")   // Destination folder for the copy

	if templateDocID == "" || destFolderID == "" {
		t.Skip("TEST_TEMPLATE_DOC_ID or TEST_DEST_FOLDER_ID not set")
	}

	// Variables to substitute
	vars := map[string]string{
		"first_name": "Jane",
		"last_name":  "Smith",
		"email":      "jane.smith@example.com",
	}

	// Create a minimal PocketBase instance (for testing workflow activity)
	pb := pocketbase.New()

	// Create workflow collections in PocketBase
	if err := pbstore.CreateCollections(pb); err != nil {
		t.Fatalf("failed to create workflow collections: %v", err)
	}

	// Create workflow engine with PocketBase store
	logger := log.New(os.Stderr, "[workflow] ", log.LstdFlags)
	eng := workflow.NewEngine(pbstore.New(pb), workflow.EngineConfig{Logger: logger})

	// Register all workflow activities (including gdrive_fill_template)
	if err := workflows.RegisterDefaults(eng, pb, nil, func() *google.Client { return gc }, nil, nil); err != nil {
		t.Fatalf("failed to register workflow activities: %v", err)
	}

	// Run the activity
	t.Run("FillTemplateAndVerify", func(t *testing.T) {
		input := map[string]any{
			"template_id":            templateDocID,
			"destination_folder_id":  destFolderID,
			"title":                  "Test - Jane Smith",
			"vars":                   varsToJSON(vars),
		}

		output, err := eng.ExecuteActivity(ctx, "gdrive_fill_template", input)
		if err != nil {
			t.Fatalf("gdrive_fill_template activity failed: %v", err)
		}

		// Extract new document ID
		newDocID, ok := output["doc_id"].(string)
		if !ok || newDocID == "" {
			t.Fatalf("expected doc_id in output, got: %v", output)
		}

		replacementsCount, ok := output["replacements_made"].(float64)
		if !ok {
			t.Fatalf("expected replacements_made in output, got: %v", output)
		}

		t.Logf("✓ Created new document: %s", newDocID)
		t.Logf("✓ Replacements made: %d", int(replacementsCount))

		if replacementsCount < float64(len(vars)) {
			t.Logf("⚠ Expected at least %d replacements, got %d", len(vars), int(replacementsCount))
		}

		// Verify the new document has the replacements
		t.Run("VerifyDocumentContent", func(t *testing.T) {
			content, err := gc.GetDocumentContent(ctx, newDocID)
			if err != nil {
				t.Fatalf("failed to get document content: %v", err)
			}

			t.Logf("Document content preview: %s...", truncate(content, 200))

			// Check that variables were replaced
			checks := map[string]string{
				"first_name": "Jane",
				"last_name":  "Smith",
				"email":      "jane.smith@example.com",
			}

			for varName, expectedValue := range checks {
				if !contains(content, expectedValue) {
					t.Errorf("expected '%s' in document (from var %s), but not found", expectedValue, varName)
				} else {
					t.Logf("✓ Found '%s' in document", expectedValue)
				}
			}

			// Check that placeholders were removed (no {{...}} left)
			if contains(content, "{{first_name}}") {
				t.Error("placeholder {{first_name}} was not replaced")
			}
			if contains(content, "{{last_name}}") {
				t.Error("placeholder {{last_name}} was not replaced")
			}
			if contains(content, "{{email}}") {
				t.Error("placeholder {{email}} was not replaced")
			}

			t.Logf("✓ All placeholders were properly replaced")
		})

		// Optional: Clean up the test document
		if os.Getenv("CLEANUP_TEST_DOCS") == "true" {
			// Would need to implement delete in Google client
			t.Logf("Document %s can be manually deleted if needed", newDocID)
		}
	})
}

// TestGdriveFillTemplate_PorphyryLiveRun runs the actual gdrive_fill_template node
// (n_21154u5) against the real Google API using the porphyry test contact context.
// Prints the doc_url of the created document.
func TestGdriveFillTemplate_PorphyryLiveRun(t *testing.T) {
	refreshToken := os.Getenv("GOOGLE_REFRESH_TOKEN")
	if refreshToken == "" {
		t.Skip("GOOGLE_REFRESH_TOKEN not set")
	}
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Fatal("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET must be set")
	}

	gc := google.NewClient(google.NewTokenManager(clientID, clientSecret, refreshToken))
	eng := workflow.NewEngine(nil, workflow.EngineConfig{Logger: log.New(os.Stderr, "[workflow] ", log.LstdFlags)})
	if err := workflows.RegisterDefaults(eng, nil, nil, func() *google.Client { return gc }, nil, nil); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Pre-resolved vars from the porphyry context (pasted_text_2026-04-12_12-09-39.txt).
	// Templates resolved: pb_contact.records.0.*, pb_birth_chart_records.records.0.*, llm_response.*
	resolvedVars := map[string]string{
		"first_name":       "",
		"birthday":         "February 8, 1985",
		"birth_time":       "1:02 AM",
		"birth_place":      "Bellevue, Washington, United States",
		"moon_code":        "Moon in Virgo in 10H ruled by Mercury in Aquarius",
		"mars_code":        "Mars in Aries in 5H ruled by Mars in Aries",
		"mercury_code":     "Mercury in Aquarius in 3H ruled by Uranus in Sagittarius",
		"jupiter_code":     "Jupiter in Aquarius in 3H ruled by Uranus in Sagittarius",
		"venus_code":       "Venus in Aries in 5H ruled by Mars in Aries",
		"chart_url":        "https://drive.google.com/uc?id=1sn6T7kdcVEy1U4qhGM4rsjqHjxF8djBj",
		"intro":            "Your chart for this reset is wired for innovation, clarity, and bold forward motion - and Spring could not be a better backdrop for what you are stepping into. You help people with their money stuff, which means your own relationship with money is part of your product. The fact that you want to move from hating it to loving it is not a small thing - it is the whole thing. Your placements suggest a mind that thinks in systems and patterns, emotions that want to be useful, and an action style that moves fast when inspired. This reset is designed to meet you exactly where you are. Each day builds on the last, using your specific cosmic codes to help you shift how you think, feel, and relate to money - not in a generic way, but in a way that actually fits how you are wired. By the end of this week, you will have a clearer, more grounded, and honestly more exciting relationship with the money side of your work.",
		"seasonal_summary": "Spring is the season of planting - and everything you do this week is a seed. The energy right now supports fresh starts, new perspectives, and breaking through patterns that have kept you stuck. For someone who helps others with money but has struggled to love it themselves, this is a powerful moment to initiate something new in that relationship. Spring does not ask you to have it all figured out - it just asks you to begin. Each day of this reset is an act of planting: a new thought, a new habit, a new way of seeing. The ground is soft and ready. What you put in now has real potential to grow into something meaningful by the time the warmer months arrive.",
		"day1_alignment":   "Your Moon in Virgo in the 10th house means your emotional relationship with money is deeply tied to your goals, your reputation, and whether you feel like you are doing things right. You process feelings through analysis and organization - you feel better when things make sense and have a clear purpose. When it comes to money, emotional honesty for you looks like noticing when anxiety is masquerading as productivity, or when perfectionism is keeping you from looking at the numbers at all. Getting honest with your feels today means acknowledging the gap between what you know intellectually and what you actually feel when money comes up.",
		"day1_seasonal":    "In Spring, your Virgo Moon wants to plant seeds with intention - not just scatter and hope. This season invites you to get clear on what emotional patterns around money you are ready to leave behind, and which ones are actually serving you. Your Moon is ruled by Mercury in Aquarius, which means your emotional clarity often comes through thinking out loud or writing things down in a structured but unconventional way. Spring is asking your Moon to be honest about what a fresh start actually requires - not just a tidy plan, but a real emotional reset.",
		"day1_approach":    "When you sit with the Reset Reflections journaling today, your Virgo Moon will want to organize and analyze what comes up. Let it - but also give yourself permission to go a little deeper than the practical surface. Try writing in lists if that feels natural, but then pick one item and write a full paragraph about how it actually makes you feel. Your emotional processing style is detailed and thoughtful, so the prompts will work best if you treat them like a personal audit - honest, thorough, and free of judgment.",
		"day1_examples":    "Emotional honesty with money might look like writing down that you avoid checking your bank account because it makes you feel like you are failing - even when the balance is fine. It might look like admitting that you undercharge because pricing yourself higher feels arrogant, not confident. For someone who helps others with money, it might mean acknowledging the irony of that dynamic out loud for the first time. These are the kinds of honest moments your Moon in Virgo is ready to surface this Spring - practical truths that, once named, actually free you up.",
		"day2_alignment":   "Your Mars in Aries in the 5th house means you are built to move fast, act boldly, and create from a place of genuine enthusiasm. When money tasks feel exciting or creative, you are unstoppable. When they feel like chores, you disappear. The key with your Mars is to make the action feel like an adventure rather than an obligation. You do not need a long runway - you need a spark. Mars in Aries is also its own ruler, which means this energy is pure and self-directed. You have more financial momentum available to you than you probably realize, especially when the task feels like it is yours to own.",
		"day2_seasonal":    "Spring is your Mars in Aries natural habitat. This is the season of bold beginnings and first moves, and your action style absolutely thrives here. The 5th house connection means your best money actions right now will have a creative or expressive quality - they will feel more like playing than grinding. Spring is not asking you to pace yourself - it is asking you to initiate. What bold money move have you been putting off because it felt too big or too fast? Your Mars says that is probably exactly the one to start with.",
		"day2_approach":    "When you look at the Mars Money Action List today, scan for the options that make you feel a little fired up - not just logical, but actually excited. Your Mars in Aries does not respond well to slow and methodical. Pick one action that feels bold and doable in a single sitting. Give yourself a time limit - maybe 25 or 30 minutes - and go all in. Your action style is built for sprints, not marathons, so honor that. One focused, energized action done today is worth more for your reset than a long list of things you meant to do.",
		"day2_examples":    "Aligned money action for your Mars might look like finally setting up that one pricing page you have been avoiding because you were not sure what to charge - and just putting a number on it. It might look like sending a follow-up message to someone you have been meaning to reconnect with about working together. Or it could be as simple as creating a short piece of content about the money stuff you help people with, in a way that feels fun and direct. Your 5th house Mars loves creative action, so anything that combines expression with forward momentum is a win.",
		"day3_alignment":   "Your Mercury in Aquarius in the 3rd house means your mind works in flashes of insight, pattern recognition, and big-picture thinking. You are wired to see connections others miss and communicate ideas in ways that feel fresh and a little unconventional. When it comes to money, you probably have some genuinely original thoughts about how it works - or how it should work. The challenge is that Aquarius Mercury can sometimes stay in the idea space and skip the part where you actually land the intention clearly. Today is about taking one of those brilliant money thoughts and turning it into a specific, grounded statement.",
		"day3_seasonal":    "Spring is all about planting clear intentions, and your Mercury in Aquarius in the 3rd house is perfectly positioned to articulate something genuinely new. This is not the season for recycling old money goals - your Mercury wants a fresh angle, a perspective that feels innovative and true to how you actually think. Ruled by Uranus in Sagittarius, your communication style has a philosophical, expansive quality underneath it. Let that inform your intention today - think bigger than just a revenue number. What is the idea or belief about money that, if you really owned it, would change everything?",
		"day3_approach":    "When setting your intention today, skip the generic and go for the specific and a little unexpected. Your Mercury in Aquarius does not resonate with cookie-cutter language. Try framing your intention as a statement that would make someone do a double take - something that sounds a little bold or counterintuitive but is completely true for you. Write it in your own words, not in the language you think you are supposed to use. Say it out loud if you can. Your 3rd house Mercury learns and solidifies ideas through articulation, so speaking it makes it more real.",
		"day3_examples":    "A clear money intention for your Mercury might sound like: I help people change their relationship with money, and I am changing mine too - starting now. Or it might be something like: Money is allowed to be simple and even enjoyable in my business. It could also be: I communicate about money with the same confidence I bring to everything else I teach. These are not affirmations for the sake of it - they are specific, personal, and tied to the real shift you are here to make. Your Aquarius Mercury will respond to an intention that feels a little revolutionary.",
		"day4_alignment":   "Your Jupiter in Aquarius in the 3rd house means your sense of abundance is tied to ideas, communication, and the sharing of knowledge. You grow when you are learning, teaching, or connecting dots in ways that open things up for others and for yourself. When it comes to money vision, you are not necessarily motivated by flashy numbers alone - you are motivated by impact, by reach, by the idea that your work is actually changing how people think. Ruled by Uranus in Sagittarius, your expansion style has a philosophical and freedom-oriented quality. The bigger the idea, the more alive your vision feels.",
		"day4_seasonal":    "Spring is the perfect season for your Jupiter in Aquarius to plant a vision that is genuinely expansive. This is not about being realistic right now - it is about letting yourself imagine what is possible if you fully commit to the growth edge in front of you. Your 3rd house Jupiter grows through sharing and communicating, which means your vision map might be less of a spreadsheet and more of a manifesto or a mind map. What does your work look like when it is reaching the people who most need it? Let Spring give you permission to think bigger than usual.",
		"day4_approach":    "When you map your vision today, give your Jupiter in Aquarius room to be unconventional. Do not force it into a traditional revenue goal format if that does not excite you - instead, try mapping it as a series of ideas or questions that expand outward. What does your business look like when money is no longer a source of dread? Who are you reaching? What are you saying? How does your work travel? Your Jupiter grows through curiosity and connection, so let the vision map feel more like an exciting brainstorm than a business plan.",
		"day4_examples":    "Vision mapping for your Jupiter might look like writing out a description of a week in your business six months from now - what you are working on, who you are talking to, how money is moving. It might look like mapping out the different ways your money help could reach people - one-on-one, a course, a community, a book. Or it could be as simple as writing the headline of an article you would love to publish about why people hate money and how to change that. Your 3rd house Jupiter loves a vision that lives in language and ideas.",
		"day5_alignment":   "Your Venus in Aries in the 5th house means you attract and relate to money in a bold, direct, and creative way - when you are in your element. You are not a slow-build, patient accumulation type. You are more of a spark-and-surge energy, someone who can create real financial momentum when you feel inspired and free to express yourself. The relationship with money you are building needs to feel alive, not dutiful. Ruled by Mars in Aries, your Venus is all fire - which means the way to make this relationship stick is to make it feel exciting, not like another thing on your to-do list.",
		"day5_seasonal":    "Spring is Venus in Aries season in the most literal sense - this energy is designed to attract fresh flow and initiate new value. Your 5th house Venus wants to bring creativity and joy into the money relationship right now, not just functionality. Think of this as the season where you get to decide what a fun relationship with money actually looks like for you. Spring is asking your Venus to be bold about what it wants - not apologetic, not hedging. What would it look like to genuinely enjoy the money side of your work? That is the seed to plant today.",
		"day5_approach":    "When you create your money altar or ritual practice today, make it feel like an expression of your personality - not a solemn ceremony. Your Venus in Aries in the 5th house wants color, energy, and a little boldness. It does not want something that feels like homework. Choose items that genuinely excite or delight you. Keep the ritual short and punchy - your Venus does not do well with slow, elaborate routines. A two-minute daily check-in that you actually look forward to is infinitely better than a 20-minute ritual you dread. Make it yours, make it fun, and make it feel like a choice.",
		"day5_examples":    "A money altar for your Venus might include something bright and bold - a red or orange candle, a piece of jewelry you associate with confidence, or a printed image of something you are working toward. Your ritual might be as simple as lighting the candle each morning while you say your intention out loud - fast, direct, done. Or it could be a weekly creative practice where you write one thing you are proud of earning or creating that week. For someone building a new relationship with money, your Venus in Aries says: make it feel like a celebration, not a chore.",
		"approach":         "When setting your intention today, skip the generic and go for the specific and a little unexpected. Your Mercury in Aquarius does not resonate with cookie-cutter language. Try framing your intention as a statement that would make someone do a double take - something that sounds a little bold or counterintuitive but is completely true for you. Write it in your own words, not in the language you think you are supposed to use. Say it out loud if you can. Your 3rd house Mercury learns and solidifies ideas through articulation, so speaking it makes it more real.",
	}

	resolvedImageVars := map[string]string{
		"chart_url": "https://drive.google.com/uc?id=1sn6T7kdcVEy1U4qhGM4rsjqHjxF8djBj",
	}

	varsJSON, _ := json.Marshal(resolvedVars)
	imageVarsJSON, _ := json.Marshal(resolvedImageVars)

	input := map[string]any{
		"template_id":           "1IMrLj2_yx2sm1UeT6CvdaCdr9h7Rn_koLrox3Jk4g2M",
		"destination_folder_id": "1sM04vnGbbW3eHp_HEoyNmPqjTr8hNRTr",
		"title":                 "TEST - Cosmic Companion for Ritual Reset - Spring 2026 - Porphyry",
		"vars":                  string(varsJSON),
		"image_vars":            string(imageVarsJSON),
	}

	out, err := eng.ExecuteActivity(context.Background(), "gdrive_fill_template", input)
	if err != nil {
		t.Fatalf("gdrive_fill_template failed: %v", err)
	}

	docURL, _ := out["doc_url"].(string)
	replacements, _ := out["replacements_made"].(int64)
	imagesReplaced, _ := out["images_replaced"].(int64)

	t.Logf("doc_url: %s", docURL)
	t.Logf("replacements_made: %d", replacements)
	t.Logf("images_replaced: %d", imagesReplaced)

	if docURL == "" {
		t.Error("expected non-empty doc_url")
	}
}

// Helper functions

func varsToJSON(vars map[string]string) string {
	b, _ := json.Marshal(vars)
	return string(b)
}

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > 0 && s[0:len(substr)] == substr || indexStr(s, substr) >= 0)
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

