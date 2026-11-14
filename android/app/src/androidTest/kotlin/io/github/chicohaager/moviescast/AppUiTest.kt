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
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
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
    private var secure: MockWebServer? = null

    @Volatile
    private var lateRefusal = false

    /** The tokens the server was told to log out, in order. */
    private val loggedOut = ArrayList<String>()

    private fun json(code: Int, body: String) =
        MockResponse.Builder().code(code).addHeader("Content-Type", "application/json").body(body).build()

    @Volatile
    private var filmPlayed = false

    @Volatile
    private var filmFavorite = false
    /** Backdrop and logo for the film, in the shape Movies sent on 2026-11-14; off for the other tests. */
    @Volatile
    private var filmPictures = false

    /** The logo then answers 404, as a server whose logo file went missing. */
    @Volatile
    private var logoMissing = false
    private val pictures get() = if (filmPictures) ""","BackdropImageTags":["bk1"],"ImageTags":{"Logo":"lg1","Primary":"pr1"}""" else ""
    private val film get() = """{"Id":"f1","Name":"Invented Film","Type":"Movie","ProductionYear":2001,"RunTimeTicks":54000000000,"Overview":"A film that does not exist.","Genres":["Drama"],"DateCreated":"2026-01-01T10:00:00Z","UserData":{"Played":$filmPlayed,"IsFavorite":$filmFavorite}$pictures}"""

    /** A small PNG as the server's picture. */
    private fun png(width: Int, height: Int): MockResponse {
        val bitmap = android.graphics.Bitmap.createBitmap(width, height, android.graphics.Bitmap.Config.ARGB_8888)
        bitmap.eraseColor(android.graphics.Color.rgb(200, 120, 40))
        val bytes = java.io.ByteArrayOutputStream().also { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }.toByteArray()
        return MockResponse.Builder().code(200).addHeader("Content-Type", "image/png").body(okio.Buffer().write(bytes)).build()
    }
    private val otherFilm = """{"Id":"f2","Name":"Second Film","Type":"Movie","ProductionYear":2003,"UserData":{"Played":false}}"""
    private val series = """{"Id":"s1","Name":"Invented Show","Type":"Series","ProductionYear":2002,"Genres":["Comedy"],"DateCreated":"2026-02-01T10:00:00Z","UserData":{"Played":true}}"""
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
        instrumentation.runOnMainSync {
            app.forgetAll()
            app.store.sort = ""
            // as on a device where the sound page was never touched
            app.store.forgetPassthrough()
            (app as TestApp).announcedByTest = emptyList()
        }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.url.encodedPath
                synchronized(paths) { paths.add("${request.method} $path") }
                return when {
                    path == "/movies/System/Info/Public" -> json(200, """{"ProductName":"Fake Movies","ServerName":"fake","Version":"10.11.0"}""")
                    path == "/movies/Users/AuthenticateByName" -> {
                        val body = request.body!!.utf8()
                        when {
                            !body.contains("\"Pw\":\"$password\"") -> json(401, "{}")
                            // a second account at the same server, with a library of its own
                            body.contains("\"Username\":\"second\"") -> json(200, """{"AccessToken":"invented-token-2","User":{"Id":"u2","Name":"second"}}""")
                            else -> json(200, """{"AccessToken":"invented-token","User":{"Id":"u1","Name":"demo"}}""")
                        }
                    }
                    path == "/movies/Users/u2/Items/Resume" -> json(200, """{"Items":[],"TotalRecordCount":0}""")
                    // answers late and refuses the token: by then the test has changed to the other login
                    path == "/movies/Users/u2/Items/f2" && lateRefusal ->
                        MockResponse.Builder().code(401).headersDelay(3, java.util.concurrent.TimeUnit.SECONDS).body("{}").build()
                    path == "/movies/Users/u2/Items" -> json(200, """{"Items":[$otherFilm],"TotalRecordCount":1}""")
                    path == "/movies/Users/u1/Items/Resume" -> json(200, """{"Items":[],"TotalRecordCount":0}""")
                    path == "/movies/Users/u1/Items" -> json(200, """{"Items":[$film,$series],"TotalRecordCount":2}""")
                    path == "/movies/Users/u1/Items/f1" -> json(200, film)
                    path == "/movies/Items/f1/Images/Backdrop/0" -> png(160, 90)
                    path == "/movies/Items/f1/Images/Logo" -> if (logoMissing) json(404, "{}") else png(80, 30)
                    path == "/movies/Users/u1/Items/s1" -> json(200, series)
                    path == "/movies/Shows/s1/Episodes" -> json(
                        200,
                        """{"Items":[${episode("e1", "One", 1, 1, true)},${episode("e2", "Two", 1, 2, false)},${episode("e3", "Three", 2, 1, false)}],"TotalRecordCount":3}""",
                    )
                    path == "/movies/Users/u1/FavoriteItems/f1" -> {
                        filmFavorite = request.method == "POST"
                        json(200, """{"IsFavorite":$filmFavorite}""")
                    }
                    path == "/movies/Users/u1/PlayedItems/f1" -> {
                        filmPlayed = request.method == "POST"
                        json(200, """{"Played":$filmPlayed}""")
                    }
                    path == "/movies/Sessions/Logout" -> {
                        val token = Regex("Token=\"([^\"]+)\"").find(request.headers["X-Emby-Authorization"].orEmpty())?.groupValues?.get(1).orEmpty()
                        synchronized(loggedOut) { loggedOut.add(token) }
                        json(204, "")
                    }
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
        InstrumentationRegistry.getInstrumentation().runOnMainSync { app.forgetAll() }
        server.close()
        secure?.close()
    }

    /**
     * The text field [tag], ready to be typed into. On a TV a field stays read-only until OK is pressed on it
     * (rememberRemoteField); the test then does what the viewer does: focus and OK on the remote.
     */
    private fun field(tag: String): SemanticsNodeInteraction {
        val node = compose.onNodeWithTag(tag)
        if (node.fetchSemanticsNode().config.getOrNull(SemanticsProperties.IsEditable) == false) {
            node.performSemanticsAction(SemanticsActions.RequestFocus)
            node.performKeyInput { pressKey(Key.DirectionCenter) }
            compose.waitForIdle()
            assertEquals("$tag after OK on the remote", true, node.fetchSemanticsNode().config.getOrNull(SemanticsProperties.IsEditable))
        }
        return node
    }

    private fun login(pw: String, user: String = "demo") {
        field("server").performTextReplacement("127.0.0.1:${server.port}")
        field("user").performTextReplacement(user)
        field("password").performTextInput(pw)
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
        // without a backdrop the page stays as it was: poster and title in text
        assertEquals(0, compose.onAllNodesWithTag("backdrop").fetchSemanticsNodes().size)
    }

    @Test
    fun aFilmWithABackdropShowsItBehindItsPageWithTheLogoAsTitle() {
        filmPictures = true
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onAllNodesWithTag("tile").onFirst().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("backdrop"), 10_000)
        compose.waitUntilAtLeastOneExists(hasTestTag("logo"), 10_000)
        compose.onNodeWithTag("logo").assertIsDisplayed()
        compose.onNodeWithContentDescription("Invented Film").assertIsDisplayed()
        compose.onNodeWithText("A film that does not exist.").assertIsDisplayed()
        compose.onNodeWithTag("play").assertIsDisplayed()
        compose.waitUntil(10_000) { synchronized(paths) { paths.contains("GET /movies/Items/f1/Images/Logo") && paths.contains("GET /movies/Items/f1/Images/Backdrop/0") } }
        compose.onNodeWithTag("back").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
    }

    @Test
    fun aLogoThatDoesNotLoadLeavesTheTitleInText() {
        filmPictures = true
        logoMissing = true
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onAllNodesWithTag("tile").onFirst().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("backdrop"), 10_000)
        compose.waitUntil(10_000) { synchronized(paths) { paths.contains("GET /movies/Items/f1/Images/Logo") } }
        compose.waitUntilAtLeastOneExists(hasText("Invented Film"), 10_000)
        assertEquals(0, compose.onAllNodesWithTag("logo").fetchSemanticsNodes().size)
    }

    @Test
    fun aWrongPasswordSaysSoAndStaysOnTheLoginPage() {
        login("wrong")
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.error_login)), 10_000)
        compose.onNodeWithTag("login").assertExists()
        assertEquals(0, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
    }

    @Test
    fun theSearchListsWhatAnswersAndATapFillsInTheAddress() {
        val movies = "http://127.0.0.1:${server.port}/movies"
        // announced: the fake server, and a box on which nothing answers as a server (404)
        (app as TestApp).announcedByTest = listOf(listOf(movies), listOf("http://127.0.0.1:${server.port}/other"))
        compose.onNodeWithTag("find").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("found"), 10_000)
        assertEquals(1, compose.onAllNodesWithTag("found").fetchSemanticsNodes().size)
        compose.onNodeWithText("fake · Fake Movies 10.11.0").assertExists()
        compose.onNodeWithTag("found").performScrollTo().performClick()
        compose.onNodeWithTag("server").assertTextContains(movies)
        field("user").performTextReplacement("demo")
        field("password").performTextInput("right")
        compose.onNodeWithTag("login").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
    }

    @Test
    fun aSearchThatFindsNothingSaysSo() {
        (app as TestApp).announcedByTest = listOf(listOf("http://127.0.0.1:${server.port}/other"))
        compose.onNodeWithTag("find").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("find_none"), 10_000)
        compose.onNodeWithText(app.getString(R.string.find_none)).assertExists()
        assertEquals(0, compose.onAllNodesWithTag("found").fetchSemanticsNodes().size)
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
    fun soundIsDecodedInTheAppUntilAFormatIsSetToPassOnWhichIsKeptAndCanBeTurnedBack() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        val all = io.github.chicohaager.moviescast.core.Passthrough.Format.entries.toSet()
        // nothing set: every format decoded in the app (Kodi's default), so a TV that cannot take DTS still has sound
        assertEquals(all, app.store.passthroughOff)
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("sound").performClick()
        val pass = app.getString(R.string.sound_pass_on)
        val decode = app.getString(R.string.sound_decode)
        for (key in listOf("ac3", "eac3", "dts", "truehd")) compose.onNodeWithTag("sound_$key").assertTextContains(decode, substring = true)
        compose.onNodeWithTag("sound_dts").performClick()
        compose.onNodeWithTag("sound_dts").assertTextContains(pass, substring = true)
        assertEquals(all - io.github.chicohaager.moviescast.core.Passthrough.Format.DTS, app.store.passthroughOff)
        // the others stay as they were
        compose.onNodeWithTag("sound_truehd").assertTextContains(decode, substring = true)
        // shown so again when the page is opened anew
        compose.onNodeWithTag("back").performClick()
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("sound").performClick()
        compose.onNodeWithTag("sound_dts").assertTextContains(pass, substring = true).performClick()
        assertEquals(all, app.store.passthroughOff)
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

    /** How far down a scrolling page is (its semantics' vertical scroll range), in pixels. */
    private fun scrolled(tag: String): Float =
        compose.onNodeWithTag(tag).fetchSemanticsNode().config[SemanticsProperties.VerticalScrollAxisRange].value()

    /** The remote's ↓ as the system sends it (a key event to the window, not to a node the test picks). */
    private fun remoteDown(times: Int) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        repeat(times) { instrumentation.sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_DPAD_DOWN) }
        compose.waitForIdle()
    }

    /** A page of text only has nothing to focus: with a remote (Fire TV, 2026-11-13) ↓ did not move it. */
    @Test
    fun thePrivacyPolicyAndALicenceScrollWithTheRemote() {
        compose.onNodeWithTag("about").performScrollTo().performClick()
        compose.onNodeWithTag("privacy").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("privacy_text"), 10_000)
        assertEquals(0f, scrolled("privacy_text"))
        remoteDown(4)
        assertTrue("privacy page after 4 × ↓: ${scrolled("privacy_text")} px", scrolled("privacy_text") > 0f)

        compose.onNodeWithTag("back").performClick()
        compose.onNodeWithTag("licences").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("licence_row"), 10_000)
        compose.onAllNodesWithTag("licence_row").onFirst().assertTextContains("FFmpeg", substring = true).performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("licence_page"), 10_000)
        assertEquals(0f, scrolled("licence_page"))
        remoteDown(4)
        assertTrue("licence page after 4 × ↓: ${scrolled("licence_page")} px", scrolled("licence_page") > 0f)
    }

    @Test
    fun sortAndFilterChangeWhatTheLibraryShows() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        assertEquals(2, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
        compose.onAllNodesWithTag("tile").onFirst().assertTextContains("Invented Film")
        // the latest first: the series is of 2002, the film of 2001
        compose.onNodeWithTag("sort").performClick()
        compose.onNodeWithTag("sort_YEAR").performClick()
        compose.onAllNodesWithTag("tile").onFirst().assertTextContains("Invented Show")
        compose.onNodeWithTag("filter").performClick()
        compose.onNodeWithTag("filter_UNWATCHED").performClick()
        assertEquals(1, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
        compose.onAllNodesWithTag("tile").onFirst().assertTextContains("Invented Film")
        compose.onNodeWithTag("filter").performClick()
        compose.onNodeWithText(app.getString(R.string.filter_genre, "Comedy")).performClick()
        compose.onAllNodesWithTag("tile").onFirst().assertTextContains("Invented Show")
        compose.onNodeWithTag("filter").performClick()
        compose.onNodeWithTag("filter_FAVOURITES").performClick()
        assertEquals(0, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
        compose.onNodeWithText(app.getString(R.string.library_none)).assertExists()
    }

    @Test
    fun aFavouriteIsSetAtTheServerAndThenFoundByTheFilter() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasTestTag("tile"), 10_000)
        compose.onNodeWithText("Invented Film").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("favourite"), 10_000)
        compose.onNodeWithTag("favourite").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.favourite_remove), substring = true), 10_000)
        assertTrue(paths.toString(), synchronized(paths) { paths.contains("POST /movies/Users/u1/FavoriteItems/f1") })
        compose.onNodeWithTag("back").performClick()
        compose.onNodeWithTag("filter").performClick()
        compose.onNodeWithTag("filter_FAVOURITES").performClick()
        assertEquals(1, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
        compose.onAllNodesWithTag("tile").onFirst().assertTextContains("Invented Film")
    }

    private fun logins() = synchronized(paths) { paths.count { it == "POST /movies/Users/AuthenticateByName" } }

    /** Logs in as "demo", then from the menu's list of logins as "second"; the library shown is then the second one's. */
    private fun loginTwice() {
        login("right")
        compose.waitUntilAtLeastOneExists(hasText("Invented Film"), 10_000)
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("accounts").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("account_add"), 10_000)
        assertEquals(1, compose.onAllNodesWithTag("account").fetchSemanticsNodes().size)
        compose.onNodeWithTag("account_add").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("login"), 10_000)
        login("right", "second")
        compose.waitUntilAtLeastOneExists(hasText("Second Film"), 10_000)
        assertEquals(1, compose.onAllNodesWithTag("tile").fetchSemanticsNodes().size)
    }

    @Test
    fun aSecondLoginIsKeptAndATapChangesBackWithoutAPassword() {
        loginTwice()
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("accounts").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("account"), 10_000)
        assertEquals(2, compose.onAllNodesWithTag("account").fetchSemanticsNodes().size)
        // the one in use carries the mark, the other does not
        compose.onNodeWithText("✓  second · fake").assertExists()
        compose.onNodeWithText("demo · fake").assertExists().performClick()
        compose.waitUntilAtLeastOneExists(hasText("Invented Film"), 10_000)
        assertEquals(0, compose.onAllNodesWithText("Second Film").fetchSemanticsNodes().size)
        assertEquals("no password was sent for the change", 2, logins())
        assertEquals(emptyList<String>(), synchronized(loggedOut) { loggedOut.toList() })
        // the choice outlives the app: what the store answers is the first login again
        assertEquals("demo", app.store.load()?.userName)
        assertEquals(2, app.store.logins().logins.size)
    }

    @Test
    fun removingTheLoginInUseLogsItOutAndLeavesTheOtherToChoose() {
        loginTwice()
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("accounts").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("account"), 10_000)
        // the second row is the login in use
        compose.onAllNodesWithTag("account_remove")[1].performClick()
        compose.waitUntil(10_000) { compose.onAllNodesWithTag("account").fetchSemanticsNodes().size == 1 }
        compose.onNodeWithText("demo · fake").assertExists()
        // no login is in use now: the list is the first page, there is nothing to go back to
        assertEquals(0, compose.onAllNodesWithTag("back").fetchSemanticsNodes().size)
        compose.waitUntil(10_000) { synchronized(loggedOut) { loggedOut.size } == 1 }
        assertEquals(listOf("invented-token-2"), synchronized(loggedOut) { loggedOut.toList() })
        compose.onNodeWithTag("account").performClick()
        compose.waitUntilAtLeastOneExists(hasText("Invented Film"), 10_000)
        assertEquals(2, logins())
    }

    @Test
    fun logOutWithASecondLoginKeptShowsTheListAndNotTheLoginPage() {
        loginTwice()
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("logout").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("account"), 10_000)
        assertEquals(1, compose.onAllNodesWithTag("account").fetchSemanticsNodes().size)
        compose.onNodeWithText("demo · fake").assertExists()
        assertEquals(0, compose.onAllNodesWithTag("login").fetchSemanticsNodes().size)
    }

    @Test
    fun aCertificateTheDeviceDoesNotTrustIsNamedOnTheLoginPage() {
        // a server with a certificate it signed itself, made here; nobody installed it on the device
        val own = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val https = MockWebServer().also { secure = it }
        https.useHttps(HandshakeCertificates.Builder().heldCertificate(own).build().sslSocketFactory())
        https.start()
        field("server").performTextReplacement("https://localhost:${https.port}")
        field("user").performTextReplacement("demo")
        field("password").performTextInput("right")
        compose.onNodeWithTag("login").performScrollTo().performClick()
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.error_certificate)), 10_000)
        compose.onNodeWithTag("login").assertExists()
        assertEquals(0, logins())
    }

    @Test
    fun aLoginFromBeforeTheBindingAtAServerTheSystemDoesNotTrustAsksToLogInAgain() {
        // a login of a build before 0.2.0 (no binding kept) at a server with a certificate it signed itself
        val own = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val https = MockWebServer().also { secure = it }
        https.useHttps(HandshakeCertificates.Builder().heldCertificate(own).build().sslSocketFactory())
        https.start()
        scenario?.close()
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            app.setSession(io.github.chicohaager.moviescast.core.Session("https://localhost:${https.port}/movies", "invented-old-token", "u9", "old"))
        }
        assertEquals("", app.store.trustOf("localhost:${https.port}"))
        scenario = ActivityScenario.launch(MainActivity::class.java)
        compose.waitUntilAtLeastOneExists(hasText(app.getString(R.string.error_certificate_login_again)), 10_000)
        assertEquals("no request got past the handshake", 0, https.requestCount)
        assertEquals("nothing was bound", "", app.store.trustOf("localhost:${https.port}"))
    }

    @Test
    fun aLoginKeptByTheAppBeforeThereWasAListIsTakenOver() {
        val prefs = app.getSharedPreferences("session-test", android.content.Context.MODE_PRIVATE)
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            app.setSession(io.github.chicohaager.moviescast.core.Session("http://192.0.2.10/movies", "invented-old-token", "u9", "old"))
        }
        val sealed = app.store.logins().logins.single().sealedToken
        // the file as 0.1.0 wrote it: one login in four keys, no list
        assertTrue(prefs.edit().remove("logins").putString("base_url", "http://192.0.2.10/movies").putString("user_id", "u9")
            .putString("user_name", "old").putString("token_sealed", sealed).commit())
        val taken = app.store.logins()
        assertEquals(1, taken.logins.size)
        assertEquals(0, taken.current)
        val session = app.store.load()
        assertEquals("http://192.0.2.10/movies", session?.baseUrl)
        assertEquals("u9", session?.userId)
        assertEquals("old", session?.userName)
        assertEquals("invented-old-token", session?.token)
        // taken over once: the four keys are gone, the list stands in their place
        assertFalse(prefs.contains("base_url") || prefs.contains("user_id") || prefs.contains("token_sealed"))
        assertTrue(prefs.contains("logins"))
    }

    @Test
    fun aRefusalThatComesAfterTheChangeOfLoginLeavesTheNewLoginAlone() {
        loginTwice()
        lateRefusal = true
        // the film page asks the server; the answer (401) is three seconds away
        compose.onNodeWithText("Second Film").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("back"), 10_000)
        compose.onNodeWithTag("back").performClick()
        compose.onNodeWithTag("menu").performClick()
        compose.onNodeWithTag("accounts").performClick()
        compose.waitUntilAtLeastOneExists(hasTestTag("account"), 10_000)
        compose.onNodeWithText("demo · fake").performClick()
        compose.waitUntilAtLeastOneExists(hasText("Invented Film"), 10_000)
        assertTrue(paths.toString(), synchronized(paths) { paths.contains("GET /movies/Users/u2/Items/f2") })
        Thread.sleep(4_000)
        compose.waitForIdle()
        compose.onNodeWithText("Invented Film").assertExists()
        assertEquals("demo", app.store.load()?.userName)
        assertEquals(2, app.store.logins().logins.size)
    }
}
