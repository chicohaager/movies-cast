package io.github.chicohaager.moviescast

import android.os.Build
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The app on a device against a fake server inside the test: login, library, film page, About,
 * licences and the diagnostics text. Invented values only. The app runs as [TestApp], so a real
 * login on the device stays as it is.
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class AppUiTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private val server = MockWebServer()
    private val paths = ArrayList<String>()
    private var password = "right"
    private lateinit var app: MoviesCastApp
    private var scenario: ActivityScenario<MainActivity>? = null

    private fun json(code: Int, body: String) =
        MockResponse.Builder().code(code).addHeader("Content-Type", "application/json").body(body).build()

    @Volatile
    private var filmPlayed = false
    private val film get() = """{"Id":"f1","Name":"Invented Film","Type":"Movie","ProductionYear":2001,"RunTimeTicks":54000000000,"Overview":"A film that does not exist.","UserData":{"Played":$filmPlayed}}"""
    private val series = """{"Id":"s1","Name":"Invented Show","Type":"Series","ProductionYear":2002}"""
    private fun episode(id: String, name: String, season: Int, number: Int, played: Boolean) =
        """{"Id":"$id","Name":"$name","Type":"Episode","SeriesId":"s1","SeriesName":"Invented Show","ParentIndexNumber":$season,"IndexNumber":$number,"UserData":{"Played":$played}}"""

    @Before
    fun start() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        app = instrumentation.targetContext.applicationContext as MoviesCastApp
        assertTrue("the tests must run as TestApp", app is TestApp)
        // Android 17 asks the person before the app may reach the home network; a dialog would stop the test.
        if (Build.VERSION.SDK_INT >= 37) {
            instrumentation.uiAutomation.grantRuntimePermission(app.packageName, "android.permission.ACCESS_LOCAL_NETWORK")
        }
        instrumentation.runOnMainSync { app.setSession(null) }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.url.encodedPath
                synchronized(paths) { paths.add("${request.method} $path") }
                return when {
                    path == "/movies/System/Info/Public" -> json(200, """{"ProductName":"Fake Movies","ServerName":"fake","Version":"10.11.0"}""")
                    path == "/movies/Users/AuthenticateByName" ->
                        if (request.body!!.utf8().contains("\"Pw\":\"$password\"")) json(200, """{"AccessToken":"invented-token","User":{"Id":"u1","Name":"demo"}}""")
                        else json(401, "{}")
                    path == "/movies/Users/u1/Items/Resume" -> json(200, """{"Items":[],"TotalRecordCount":0}""")
                    path == "/movies/Users/u1/Items" -> json(200, """{"Items":[$film,$series],"TotalRecordCount":2}""")
                    path == "/movies/Users/u1/Items/f1" -> json(200, film)
                    path == "/movies/Users/u1/Items/s1" -> json(200, series)
                    path == "/movies/Shows/s1/Episodes" -> json(
                        200,
                        """{"Items":[${episode("e1", "One", 1, 1, true)},${episode("e2", "Two", 1, 2, false)},${episode("e3", "Three", 2, 1, false)}],"TotalRecordCount":3}""",
                    )
                    path == "/movies/Users/u1/PlayedItems/f1" -> {
                        filmPlayed = request.method == "POST"
                        json(200, """{"Played":$filmPlayed}""")
                    }
                    path == "/movies/Sessions/Logout" -> json(204, "")
                    else -> json(404, "{}")
                }
            }
        }
        server.start()
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    @After
    fun stop() {
        scenario?.close()
        InstrumentationRegistry.getInstrumentation().runOnMainSync { app.setSession(null) }
        server.close()
    }

    private fun login(pw: String) {
        compose.onNodeWithTag("server").performTextReplacement("127.0.0.1:${server.port}")
        compose.onNodeWithTag("user").performTextReplacement("demo")
        compose.onNodeWithTag("password").performTextInput(pw)
        compose.onNodeWithTag("login").performScrollTo().performClick()
    }

    @Test
    fun loginShowsTheLibraryAndTheFilmPage() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onNodeWithText("Invented Film").assertIsDisplayed()
        compose.onAllNodesWithTag("tile").onFirst().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("play"), 10_000)
        compose.onNodeWithText("A film that does not exist.").assertIsDisplayed()
        val seen = synchronized(paths) { paths.toList() }
        assertTrue(seen.toString(), seen.contains("POST /movies/Users/AuthenticateByName"))
        assertTrue(seen.toString(), seen.contains("GET /movies/Users/u1/Items/f1"))
    }

    @Test
    fun aWrongPasswordSaysSoAndStaysOnTheLoginPage() {
        login("wrong")
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.error_login)), 10_000)
        compose.onNodeWithTag("login").assertExists()
        assertEquals(0, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
    }

    @Test
    fun aboutNamesTheVersionAndTheLicencesNameFfmpegWithTheLgpl() {
        compose.onNodeWithTag("about").performScrollTo().performClick()
        compose.onNodeWithTag("version").assertTextContains(BuildConfig.VERSION_NAME, substring = true)
        compose.onNodeWithTag("licences").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("licence_row"), 10_000)
        compose.onAllNodesWithTag("licence_row").onFirst().assertTextContains("FFmpeg", substring = true).performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("licence_text"), 10_000)
        compose.onNodeWithTag("licence_text").assertTextContains("GNU LESSER GENERAL PUBLIC LICENSE", substring = true)
    }

    @Test
    fun theDiagnosticsTextIsShownAndCarriesNoAddress() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("about").performClick()
        compose.onNodeWithTag("diagnostics").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("report"), 10_000)
        val text = compose.onNodeWithTag("report").fetchSemanticsNode().config[SemanticsProperties.Text].joinToString("") { it.text }
        assertTrue(text, text.startsWith("app: ${BuildConfig.VERSION_NAME}"))
        assertTrue(text, text.contains("server: Fake Movies 10.11.0"))
        assertTrue(text, text.contains("decoders ("))
        assertFalse(text, text.contains("127.0.0.1"))
        assertFalse(text, text.contains("demo"))
        assertFalse(text, text.contains("invented-token"))
    }

    @Test
    fun aSeriesShowsItsSeasonsAndWhichEpisodesWereWatched() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasText("Invented Show"), 10_000)
        compose.onNodeWithText("Invented Show").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("episode"), 10_000)
        assertEquals(2, compose.onAllNodesWithTag("season").fetchSemanticsNodes().size)
        compose.onNodeWithText(app.getString(R.string.season_label, 2)).assertExists()
        compose.onNodeWithText("✓  " + app.getString(R.string.episode_label, 1, 1, "One")).assertExists()
        compose.onNodeWithText(app.getString(R.string.episode_label, 1, 2, "Two")).assertExists()
    }

    @Test
    fun markAsWatchedAsksTheServerAndTheButtonTurns() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onNodeWithText("Invented Film").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("played"), 10_000)
        compose.onNodeWithTag("played").assertTextContains(app.getString(R.string.mark_watched)).performClick()
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.mark_unwatched), substring = true), 10_000)
        assertTrue(paths.toString(), synchronized(paths) { paths.contains("POST /movies/Users/u1/PlayedItems/f1") })
        compose.onNodeWithTag("played").performClick()
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.mark_watched)), 10_000)
        assertTrue(paths.toString(), synchronized(paths) { paths.contains("DELETE /movies/Users/u1/PlayedItems/f1") })
    }

    @Test
    fun thePrivacyPolicyIsAPageOfTheApp() {
        compose.onNodeWithTag("about").performScrollTo().performClick()
        compose.onNodeWithTag("privacy").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("privacy_text"), 10_000)
        compose.onNodeWithText("no analytics", substring = true).assertExists()
        compose.onNodeWithText("Movies-Cast for Android", substring = true).assertExists()
    }
}
