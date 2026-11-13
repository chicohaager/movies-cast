package io.github.chicohaager.moviescast.core

import java.io.IOException
import kotlinx.coroutines.runBlocking
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/** Invented values only: no address, account or token of a real installation. */
class MoviesClientTest {
    private val server = MockWebServer()
    private val seen = ArrayList<RecordedRequest>()
    private var routes: (RecordedRequest) -> MockResponse = { json(404, "{}") }
    private val client = MoviesClient(OkHttpClient(), Device("dev-1", "Test \"Phone\", large", "0.0.1"))

    private fun json(code: Int, body: String) =
        MockResponse.Builder().code(code).addHeader("Content-Type", "application/json").body(body).build()

    private val base get() = server.url("/movies").toString()
    private val session get() = Session(base, "tok-secret-123", "user-1", "demo")

    @Before
    fun start() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                synchronized(seen) { seen.add(request) }
                return routes(request)
            }
        }
        server.start()
    }

    @After
    fun stop() = server.close()

    @Test
    fun publicInfoNeedsNoTokenAndReadsTheNames() = runBlocking {
        routes = { json(200, """{"ProductName":"ZimaOS Movies","ServerName":"ZimaOS","Version":"10.11.0","Extra":1}""") }
        val info = client.publicInfo(base)
        assertEquals(ServerInfo("ZimaOS Movies", "ZimaOS", "10.11.0"), info)
        val request = seen.single()
        assertEquals("/movies/System/Info/Public", request.url.encodedPath)
        assertFalse(request.headers["X-Emby-Authorization"]!!.contains("Token"))
    }

    @Test
    fun loginSendsTheAccountAndKeepsTheToken() = runBlocking {
        routes = { json(200, """{"AccessToken":"tok-secret-123","User":{"Id":"user-1","Name":"demo"}}""") }
        val s = client.login(base, "demo", "pw \"quoted\"")
        assertEquals(session, s)
        val request = seen.single()
        assertEquals("POST", request.method)
        assertEquals("/movies/Users/AuthenticateByName", request.url.encodedPath)
        assertEquals("""{"Username":"demo","Pw":"pw \"quoted\""}""", request.body!!.utf8())
    }

    @Test
    fun loginAnswerWithoutTokenIsAnError() = runBlocking {
        routes = { json(200, """{"User":{"Id":"user-1"}}""") }
        try {
            client.login(base, "demo", "pw")
            fail("a login without a token was accepted")
        } catch (e: IOException) {
            assertEquals("login answer without token", e.message)
        }
    }

    @Test
    fun wrongPasswordIsUnauthorizedAndOtherErrorsDoNotQuoteTheBody() = runBlocking {
        routes = { json(401, """{"echo":"pw"}""") }
        try {
            client.login(base, "demo", "pw")
            fail("401 did not throw")
        } catch (_: UnauthorizedException) {
        }
        routes = { json(500, """{"echo":"pw-in-body"}""") }
        try {
            client.login(base, "demo", "pw")
            fail("500 did not throw")
        } catch (e: ServerException) {
            assertEquals(500, e.status)
            assertFalse(e.message!!.contains("pw-in-body"))
        }
    }

    @Test
    fun theTokenRidesInTheHeaderNeverInAUrl() = runBlocking {
        routes = { json(200, """{"Items":[],"TotalRecordCount":0}""") }
        client.library(session)
        client.resume(session)
        client.reportProgress(session, PlayState("item-1", "play-1", 90, false))
        assertEquals(3, seen.size)
        for (request in seen) {
            assertTrue(request.headers["X-Emby-Authorization"]!!.contains("Token=\"tok-secret-123\""))
            assertFalse(request.url.toString().contains("tok-secret-123"))
        }
        for (url in listOf(client.streamUrl(session, "item-1"), client.posterUrl(session, "item-1", 300))) {
            assertFalse(url.contains("tok-secret-123"))
        }
        assertEquals("$base/Videos/item-1/stream?static=true", client.streamUrl(session, "item-1"))
        assertEquals("$base/Items/item-1/Images/Primary?maxWidth=300", client.posterUrl(session, "item-1", 300))
    }

    @Test
    fun theDeviceNameCannotBreakTheHeader() {
        val header = client.authHeader("t")
        assertEquals(
            "MediaBrowser Client=\"Movies-Cast Android\", Device=\"Test Phone  large\", DeviceId=\"dev-1\", Version=\"0.0.1\", Token=\"t\"",
            header,
        )
    }

    @Test
    fun aRedirectIsNotFollowedWithTheToken() = runBlocking {
        routes = { request ->
            if (request.url.encodedPath.endsWith("/Items/Resume")) {
                MockResponse.Builder().code(307).addHeader("Location", server.url("/elsewhere").toString()).build()
            } else {
                json(200, """{"Items":[{"Id":"x","Name":"followed"}]}""")
            }
        }
        try {
            client.resume(session)
            fail("the redirect was followed")
        } catch (e: ServerException) {
            assertEquals(307, e.status)
        }
        assertEquals(1, seen.size)
    }

    @Test
    fun theLibraryIsReadInPagesAndMapped() = runBlocking {
        val first = (1..MoviesClient.PAGE_SIZE).joinToString(",") { """{"Id":"m$it","Name":"Film $it","Type":"Movie"}""" }
        routes = { request ->
            when (request.url.queryParameter("StartIndex")) {
                "0" -> json(200, """{"Items":[$first],"TotalRecordCount":201}""")
                "200" -> json(
                    200,
                    """{"Items":[{"Id":"last","Name":"Last","Type":"Movie","ProductionYear":1999,
                    "RunTimeTicks":72000000000,"ImageTags":{"Primary":"abc"},
                    "UserData":{"PlaybackPositionTicks":6000000000,"Played":false},
                    "MediaSources":[{"Container":"mkv","Size":4096,"MediaStreams":[
                      {"Type":"Video","Codec":"hevc","Width":3840,"Height":2160,"Index":0},
                      {"Type":"Audio","Codec":"eac3","Language":"ger","DisplayTitle":"German 5.1","Channels":6,"Index":1},
                      {"Type":"Subtitle","Codec":"subrip","Language":"eng","Index":2}]}]}],"TotalRecordCount":201}""",
                )
                else -> json(500, "{}")
            }
        }
        val items = client.library(session)
        assertEquals(201, items.size)
        assertEquals(2, seen.size)
        assertEquals("/movies/Users/user-1/Items", seen[0].url.encodedPath)
        assertEquals("Movie,Series", seen[0].url.queryParameter("IncludeItemTypes"))
        assertEquals("200", seen[0].url.queryParameter("Limit"))
        val last = items.last()
        assertEquals(1999, last.year)
        assertEquals(7200L, last.runtimeSeconds)
        assertEquals(600L, last.resumeSeconds)
        assertTrue(last.hasPoster)
        assertTrue(last.resumable)
        assertEquals("mkv", last.container)
        assertEquals("hevc", last.videoCodec)
        assertEquals(3840, last.width)
        assertEquals(4096L, last.sizeBytes)
        assertEquals(listOf(Track(1, "eac3", "ger", "German 5.1", 6)), last.audio)
        assertEquals(listOf(Track(2, "subrip", "eng", "eng", 0)), last.subtitles)
        assertFalse(items.first().hasPoster)
    }

    @Test
    fun aServerThatIgnoresPagingStopsWithAnError() = runBlocking {
        routes = { json(200, """{"Items":[{"Id":"a","Name":"A"},{"Id":"b","Name":"B"}],"TotalRecordCount":5}""") }
        try {
            client.library(session)
            fail("a repeated page was accepted as the whole library")
        } catch (e: IOException) {
            assertEquals("got 2 of 5 items", e.message)
        }
        assertEquals(2, seen.size)
    }

    @Test
    fun progressIsReportedInTicksAsDirectPlay() = runBlocking {
        routes = { MockResponse.Builder().code(204).build() }
        client.reportStart(session, PlayState("item-1", "play-1", 0, false))
        client.reportProgress(session, PlayState("item-1", "play-1", 90, true))
        client.reportStopped(session, PlayState("item-1", "play-1", 95, false))
        assertEquals(
            listOf("/movies/Sessions/Playing", "/movies/Sessions/Playing/Progress", "/movies/Sessions/Playing/Stopped"),
            seen.map { it.url.encodedPath },
        )
        assertEquals(
            """{"ItemId":"item-1","MediaSourceId":"item-1","PlaySessionId":"play-1","PositionTicks":900000000,"IsPaused":true,"PlayMethod":"DirectPlay","CanSeek":true}""",
            seen[1].body!!.utf8(),
        )
    }

    @Test
    fun browsingAndPlayingTheOriginalNeverCallsPlaybackInfo() = runBlocking {
        routes = { request ->
            if (request.url.encodedPath.contains("/Items/item-1")) {
                json(200, """{"Id":"item-1","Name":"One","Type":"Movie","MediaStreams":[{"Type":"Audio","Codec":"ac3","Index":1}]}""")
            } else {
                json(200, """{"Items":[],"TotalRecordCount":0}""")
            }
        }
        val item = client.item(session, "item-1")
        client.library(session)
        client.resume(session)
        client.episodes(session, "series-1")
        client.reportStart(session, PlayState("item-1", "play-1", 0, false))
        assertEquals(listOf(Track(1, "ac3", "", "", 0)), item.audio)
        assertEquals("Overview,MediaSources,MediaStreams,Genres", seen[0].url.queryParameter("Fields"))
        assertTrue(seen.none { it.url.encodedPath.contains("PlaybackInfo") })
    }

    @Test
    fun aConvertedStreamIsAskedWithAProfileThatPlaysNothingDirectly() = runBlocking {
        routes = { json(200, """{"MediaSources":[{"Id":"item-1"},{"TranscodingUrl":"/movies/Videos/item-1/master.m3u8?VideoCodec=h264&api_key=server-made"}]}""") }
        val url = client.transcodeUrl(session, "item-1")
        assertEquals(server.url("/movies/Videos/item-1/master.m3u8?VideoCodec=h264&api_key=server-made").toString(), url)
        val request = seen.single()
        assertEquals("POST", request.method)
        assertEquals("/movies/Items/item-1/PlaybackInfo", request.url.encodedPath)
        assertEquals("user-1", request.url.queryParameter("UserId"))
        assertEquals(
            """{"DeviceProfile":{"MaxStreamingBitrate":40000000,"DirectPlayProfiles":[],"TranscodingProfiles":[{"Container":"ts","Type":"Video","VideoCodec":"h264","AudioCodec":"aac","Protocol":"hls","Context":"Streaming"}],"CodecProfiles":[],"SubtitleProfiles":[]}}""",
            request.body!!.utf8(),
        )
    }

    @Test
    fun aConvertedStreamIsLimitedToThePictureTheDecoderTakes() = runBlocking {
        routes = { json(200, """{"MediaSources":[{"TranscodingUrl":"/movies/Videos/item-1/master.m3u8"}]}""") }
        client.transcodeUrl(session, "item-1", 1920, 1088)
        val profile = seen.single().body!!.utf8()
        assertTrue(profile, profile.contains(""""MaxStreamingBitrate":10000000,"""))
        assertEquals(listOf(40_000_000, 40_000_000, 10_000_000, 4_000_000), listOf(0, 4096, 1920, 1280).map(MoviesClient::bitrateFor))
        assertTrue(profile, profile.contains(""""CodecProfiles":[{"Type":"Video","Codec":"h264","Conditions":[{"Condition":"LessThanEqual","Property":"Width","Value":"1920","IsRequired":false},{"Condition":"LessThanEqual","Property":"Height","Value":"1088","IsRequired":false}]}]"""))
    }

    @Test
    fun noConvertedStreamIsNull() = runBlocking {
        routes = { json(200, """{"MediaSources":[{"Id":"item-1"}],"ErrorCode":"NoCompatibleStream"}""") }
        assertEquals(null, client.transcodeUrl(session, "item-1"))
    }

    @Test
    fun theServersStreamPathIsResolvedAgainstTheServerOnly() {
        // Movies behind the ZimaOS gateway answers with its prefix, Jellyfin from its own root.
        assertEquals("http://nas.example/movies/Videos/a/master.m3u8?x=1", MoviesClient.resolve("http://nas.example/movies", "/movies/Videos/a/master.m3u8?x=1"))
        assertEquals("http://nas.example:8096/videos/a/master.m3u8", MoviesClient.resolve("http://nas.example:8096", "/videos/a/master.m3u8"))
        assertEquals("https://nas.example/jf/videos/a/master.m3u8", MoviesClient.resolve("https://nas.example/jf/", "/videos/a/master.m3u8"))
        assertEquals("http://nas.example/movies/moviesX/a", MoviesClient.resolve("http://nas.example/movies", "/moviesX/a"))
        assertEquals(null, MoviesClient.resolve("http://nas.example/movies", "http://elsewhere.example/a.m3u8"))
        assertEquals(null, MoviesClient.resolve("http://nas.example/movies", "//elsewhere.example/a.m3u8"))
        assertEquals(null, MoviesClient.resolve("http://nas.example/movies", "videos/a.m3u8"))
        assertEquals("http://nas.example/movies/Videos/a.m3u8?next=http://x.example/", MoviesClient.resolve("http://nas.example/movies", "/movies/Videos/a.m3u8?next=http://x.example/"))
    }

    @Test
    fun whatAPersonTypesBecomesBaseUrlsToTry() {
        assertEquals(
            listOf("http://nas.example/movies", "http://nas.example", "http://nas.example:8096"),
            MoviesClient.candidates(" nas.example/ "),
        )
        assertEquals(listOf("http://192.0.2.10:8096/movies", "http://192.0.2.10:8096"), MoviesClient.candidates("192.0.2.10:8096"))
        assertEquals(listOf("https://nas.example/movies"), MoviesClient.candidates("https://nas.example/movies/"))
        assertEquals(emptyList<String>(), MoviesClient.candidates("  "))
        assertEquals(emptyList<String>(), MoviesClient.candidates("http://"))
    }

    @Test
    fun discoverSkipsAPageThatIsNotTheServer() = runBlocking {
        routes = { request ->
            if (request.url.encodedPath == "/movies/System/Info/Public") {
                MockResponse.Builder().code(200).addHeader("Content-Type", "text/html").body("<html>dashboard</html>").build()
            } else {
                json(200, """{"ProductName":"Jellyfin Server","Version":"10.11.0"}""")
            }
        }
        val (url, info) = client.discover(server.url("/").toString())
        assertEquals(server.url("/").toString().trimEnd('/'), url)
        assertEquals("Jellyfin Server", info.product)
    }

    @Test
    fun discoverRefusesAnAnswerWithoutAVersion() = runBlocking {
        routes = { json(200, "{}") }
        try {
            client.discover(server.url("/").toString())
            fail("an answer without a version was taken for a server")
        } catch (e: IOException) {
            assertTrue(e.message!!.startsWith("no server info at "))
        }
        assertEquals(2, seen.size)
    }

    @Test
    fun aSessionDoesNotPrintItsToken() {
        assertFalse(session.toString().contains("tok-secret-123"))
    }

    @Test
    fun resumeIsOfferedOnlyInsideTheFilm() {
        val film = MediaItem("i", "n", "Movie", runtimeSeconds = 6000)
        assertFalse(film.copy(resumeSeconds = 5).resumable)
        assertTrue(film.copy(resumeSeconds = 600).resumable)
        assertFalse(film.copy(resumeSeconds = 5990).resumable)
        assertFalse(MediaItem("s", "n", "Series").playable)
    }
}
