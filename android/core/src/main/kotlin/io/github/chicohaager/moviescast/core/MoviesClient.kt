package io.github.chicohaager.moviescast.core

import java.io.IOException
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLPeerUnverifiedException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.addJsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/** The token is missing, expired or logged out. */
class UnauthorizedException : IOException("unauthorized")

/** The server answered with a status that is not 2xx. The body is never quoted: on a login it could echo the request. */
class ServerException(val status: Int, val path: String) : IOException("HTTP $status for $path")

/** The answer was not the JSON this client expects (a login page, a proxy's error page). */
class UnexpectedAnswerException : IOException("unexpected answer from the server")

/**
 * The device does not trust the server's certificate (self-signed, or signed by an authority the
 * device does not know), or the certificate is made out for another name than the address.
 */
open class UntrustedCertificateException(message: String = "the server's certificate is not trusted") : IOException(message)

/** The server's certificate is not the one it was logged in with ([ScopedTrust]); logging in again takes the new one. */
class CertificateChangedException : UntrustedCertificateException("the server's certificate is not the one of the login")

/** Who is asking; goes into the `X-Emby-Authorization` header of every request. */
data class Device(val id: String, val name: String, val appVersion: String)

/**
 * Talks to IceWhale Movies (ZimaOS) through its Jellyfin-compatible API, which the ZimaOS gateway
 * serves under `/movies`, and to a Jellyfin server the same way.
 *
 * Kept from movies-cast's Go client (measured against Movies 0.3.0 on 2026-10-28):
 * routes are case-sensitive; the original file comes from `/Videos/{id}/stream?static=true` with
 * Range support; `PlaybackInfo` makes Movies start a transcode, so this client never calls it;
 * the token rides in a header and never in a URL, and no redirect is followed with it.
 */
class MoviesClient(http: OkHttpClient, private val device: Device) {

    private val http: OkHttpClient = http.newBuilder().followRedirects(false).followSslRedirects(false).build()
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false; coerceInputValues = true }

    /** The value of the `X-Emby-Authorization` header. */
    fun authHeader(token: String?): String {
        fun clean(s: String) = s.replace("\"", "").replace(",", " ")
        val base = "MediaBrowser Client=\"Movies-Cast Android\", Device=\"${clean(device.name)}\", " +
            "DeviceId=\"${clean(device.id)}\", Version=\"${clean(device.appVersion)}\""
        return if (token.isNullOrEmpty()) base else "$base, Token=\"$token\""
    }

    suspend fun publicInfo(baseUrl: String): ServerInfo {
        val dto = decode<PublicInfoDto>(call(baseUrl, null, "GET", "/System/Info/Public", null))
        return ServerInfo(dto.productName, dto.serverName, dto.version)
    }

    /**
     * Finds the server behind what a person typed: `nas.local`, `192.0.2.10:8096`,
     * `https://nas.example/movies`. Returns the base URL that answered and its info.
     */
    suspend fun discover(input: String): Pair<String, ServerInfo> {
        val errors = ArrayList<IOException>()
        for (candidate in candidates(input)) {
            try {
                val info = publicInfo(candidate)
                if (info.version.isNotEmpty()) return candidate to info
                errors.add(IOException("no server info at $candidate"))
            } catch (e: IOException) {
                errors.add(e)
            }
        }
        throw reason(errors)
    }

    /**
     * Of what a search of the home network brought ([ServerSearch]), the servers that answer:
     * [announced] holds one list of base URLs per server, the first that answers within
     * [timeoutMs] counts. All are asked at the same time.
     */
    suspend fun confirm(announced: List<List<String>>, timeoutMs: Long = 3000): List<FoundServer> = coroutineScope {
        val quick = http.newBuilder().callTimeout(timeoutMs, TimeUnit.MILLISECONDS).build()
        announced.map { candidates ->
            async {
                candidates.map { candidate ->
                    async {
                        try {
                            val dto = decode<PublicInfoDto>(call(url(candidate, "/System/Info/Public") {}, null, "GET", null, quick))
                            if (dto.version.isEmpty()) null else FoundServer(candidate, dto.serverName, dto.productName, dto.version)
                        } catch (_: IOException) {
                            null
                        } catch (_: IllegalArgumentException) {
                            null
                        }
                    }
                }.awaitAll().firstOrNull { it != null }
            }
        }.awaitAll().filterNotNull().distinctBy { it.address }
    }

    /**
     * Logs in. [chain], when given, gets the certificates the server showed for this answer (none over
     * plain HTTP): the login is bound to what issued them ([ScopedTrust.bindingOf]). Read from the
     * answer, not from the handshake, as a TLS session that is taken up again has no handshake.
     */
    suspend fun login(baseUrl: String, user: String, password: String, chain: MutableList<X509Certificate>? = null): Session {
        val body = JsonObject(mapOf("Username" to JsonPrimitive(user), "Pw" to JsonPrimitive(password))).toString()
        val dto = decode<AuthDto>(call(url(baseUrl, "/Users/AuthenticateByName") {}, null, "POST", body, chain = chain))
        if (dto.accessToken.isEmpty() || dto.user.id.isEmpty()) throw IOException("login answer without token")
        return Session(baseUrl, dto.accessToken, dto.user.id, dto.user.name.ifEmpty { user })
    }

    suspend fun logout(s: Session) {
        call(s.baseUrl, s.token, "POST", "/Sessions/Logout", "")
    }

    /** Every film and series of the user, sorted by name, read in pages. */
    suspend fun library(s: Session, types: String = "Movie,Series"): List<MediaItem> =
        items(s, "/Users/${seg(s.userId)}/Items") {
            addQueryParameter("Recursive", "true")
            addQueryParameter("IncludeItemTypes", types)
            addQueryParameter("SortBy", "SortName")
            addQueryParameter("Fields", LIBRARY_FIELDS)
        }

    /** The episodes of a series, in order. */
    suspend fun episodes(s: Session, seriesId: String): List<MediaItem> =
        items(s, "/Shows/${seg(seriesId)}/Episodes") {
            addQueryParameter("UserId", s.userId)
            addQueryParameter("Fields", LIBRARY_FIELDS)
        }

    /** Films with a resume point, most recently watched first (the server's order). */
    suspend fun resume(s: Session): List<MediaItem> {
        val url = url(s.baseUrl, "/Users/${seg(s.userId)}/Items/Resume") {
            addQueryParameter("MediaTypes", "Video")
            addQueryParameter("Fields", "Overview,MediaSources")
        }
        return decode<ItemsDto>(call(url, s.token, "GET", null)).items.map { it.toItem() }
    }

    /** One item with its audio and subtitle tracks. */
    suspend fun item(s: Session, id: String): MediaItem {
        val url = url(s.baseUrl, "/Users/${seg(s.userId)}/Items/${seg(id)}") {
            addQueryParameter("Fields", "Overview,MediaSources,MediaStreams,Genres")
        }
        val dto = decode<ItemDto>(call(url, s.token, "GET", null))
        if (dto.id.isEmpty()) throw ServerException(404, "/Users/…/Items/…")
        return dto.toItem()
    }

    /** The original file, unchanged; the caller sends [authHeader] with the request. */
    fun streamUrl(s: Session, id: String): String =
        url(s.baseUrl, "/Videos/${seg(id)}/stream") { addQueryParameter("static", "true") }.toString()

    /**
     * The subtitle files lying next to the film that the player can read (SRT, WebVTT, ASS/SSA), with
     * the address the server hands each out at. As Jellyfin's Android TV app builds it for external
     * players: `DeliveryUrl` is empty without `PlaybackInfo`, so the format comes from the file's name.
     * Measured against Jellyfin 10.11.0 (2026-11-13): `…/Subtitles/0/Stream.srt` answered the file.
     * Movies 0.4.5 serves such a file at the same address but does not name it in the item's streams
     * (only `PlaybackInfo` does, which this client does not call), so with Movies this list is empty.
     */
    fun subtitleFiles(s: Session, item: MediaItem): List<SubtitleFile> =
        item.subtitles.filter { it.external }.mapNotNull { track ->
            val format = when (track.format.ifEmpty { track.codec }.lowercase()) {
                "srt", "subrip" -> "srt"
                "vtt", "webvtt" -> "vtt"
                "ass" -> "ass"
                "ssa" -> "ssa"
                else -> return@mapNotNull null
            }
            val address = url(s.baseUrl, "/Videos/${seg(item.id)}/${seg(item.sourceId.ifEmpty { item.id })}/Subtitles/${track.index}/Stream.$format") {}
            SubtitleFile(address.toString(), format, track.language, track.title)
        }

    /** Marks an item as watched or as not watched; the server's "continue watching" forgets it either way. */
    suspend fun setPlayed(s: Session, id: String, played: Boolean) {
        call(s.baseUrl, s.token, if (played) "POST" else "DELETE", "/Users/${seg(s.userId)}/PlayedItems/${seg(id)}", if (played) "" else null)
    }

    /** Puts an item among the user's favourites or takes it out. */
    suspend fun setFavorite(s: Session, id: String, favorite: Boolean) {
        call(s.baseUrl, s.token, if (favorite) "POST" else "DELETE", "/Users/${seg(s.userId)}/FavoriteItems/${seg(id)}", if (favorite) "" else null)
    }

    fun posterUrl(s: Session, id: String, maxWidth: Int): String =
        url(s.baseUrl, "/Items/${seg(id)}/Images/Primary") { addQueryParameter("maxWidth", maxWidth.toString()) }.toString()

    /**
     * A stream the server converts to H.264 and AAC (HLS), for a device that cannot decode the
     * original; null when the server offers none. This is the one call to `PlaybackInfo`: it makes
     * the box start converting, so it is asked only after the original failed on the device.
     * The URL the server answers carries its own key in the query (Movies and Jellyfin both do
     * that); it goes to the player only and must not be logged.
     */
    suspend fun transcodeUrl(s: Session, id: String, maxWidth: Int = 0, maxHeight: Int = 0): String? {
        val body = buildJsonObject {
            putJsonObject("DeviceProfile") {
                put("MaxStreamingBitrate", bitrateFor(maxWidth))
                putJsonArray("DirectPlayProfiles") {}
                putJsonArray("TranscodingProfiles") {
                    addJsonObject {
                        put("Container", "ts")
                        put("Type", "Video")
                        put("VideoCodec", "h264")
                        put("AudioCodec", "aac")
                        put("Protocol", "hls")
                        put("Context", "Streaming")
                    }
                }
                putJsonArray("CodecProfiles") {
                    // The largest picture the device's H.264 decoder takes, as Jellyfin's Android TV app sends it.
                    if (maxWidth > 0 && maxHeight > 0) addJsonObject {
                        put("Type", "Video")
                        put("Codec", "h264")
                        putJsonArray("Conditions") {
                            for ((property, value) in listOf("Width" to maxWidth, "Height" to maxHeight)) addJsonObject {
                                put("Condition", "LessThanEqual")
                                put("Property", property)
                                put("Value", value.toString())
                                put("IsRequired", false)
                            }
                        }
                    }
                }
                putJsonArray("SubtitleProfiles") {}
            }
        }.toString()
        val url = url(s.baseUrl, "/Items/${seg(id)}/PlaybackInfo") { addQueryParameter("UserId", s.userId) }
        val answer = decode<PlaybackInfoDto>(call(url, s.token, "POST", body))
        val relative = answer.sources.firstOrNull { it.transcodingUrl.isNotEmpty() }?.transcodingUrl ?: return null
        return resolve(s.baseUrl, relative)
    }

    suspend fun reportStart(s: Session, p: PlayState) = report(s, "/Sessions/Playing", p)

    suspend fun reportProgress(s: Session, p: PlayState) = report(s, "/Sessions/Playing/Progress", p)

    suspend fun reportStopped(s: Session, p: PlayState) = report(s, "/Sessions/Playing/Stopped", p)

    private suspend fun report(s: Session, path: String, p: PlayState) {
        val body = JsonObject(
            mapOf(
                "ItemId" to JsonPrimitive(p.itemId),
                "MediaSourceId" to JsonPrimitive(p.itemId),
                "PlaySessionId" to JsonPrimitive(p.playSessionId),
                "PositionTicks" to JsonPrimitive(p.positionSeconds * TICKS_PER_SECOND),
                "IsPaused" to JsonPrimitive(p.paused),
                "PlayMethod" to JsonPrimitive("DirectPlay"),
                "CanSeek" to JsonPrimitive(true),
            ),
        ).toString()
        call(s.baseUrl, s.token, "POST", path, body)
    }

    /** An answer that is not the expected JSON (a login page, a proxy's error page) is an I/O error, not a crash. */
    private inline fun <reified T> decode(text: String): T =
        try {
            json.decodeFromString<T>(text)
        } catch (e: SerializationException) {
            throw UnexpectedAnswerException()
        }

    private suspend fun items(s: Session, path: String, query: HttpUrl.Builder.() -> Unit): List<MediaItem> {
        val out = ArrayList<MediaItem>()
        val seen = HashSet<String>()
        var start = 0
        while (true) {
            val url = url(s.baseUrl, path) {
                query()
                addQueryParameter("StartIndex", start.toString())
                addQueryParameter("Limit", PAGE_SIZE.toString())
            }
            val page = decode<ItemsDto>(call(url, s.token, "GET", null))
            var fresh = 0
            for (dto in page.items) {
                if (seen.add(dto.id)) {
                    fresh++
                    out.add(dto.toItem())
                }
            }
            start += page.items.size
            // A server that ignores StartIndex answers the first page again: stop instead of looping.
            if (page.items.isEmpty() || fresh == 0 || start >= page.total) {
                if (page.total > out.size) throw IOException("got ${out.size} of ${page.total} items")
                return out
            }
        }
    }

    private suspend fun call(baseUrl: String, token: String?, method: String, path: String, body: String?): String =
        call(url(baseUrl, path) {}, token, method, body)

    private suspend fun call(
        url: HttpUrl,
        token: String?,
        method: String,
        body: String?,
        http: OkHttpClient = this.http,
        chain: MutableList<X509Certificate>? = null,
    ): String =
        withContext(Dispatchers.IO) {
            val request = Request.Builder().url(url)
                .header("X-Emby-Authorization", authHeader(token))
                .header("Accept", "application/json")
                .method(method, body?.toRequestBody(JSON))
                .build()
            try {
                http.newCall(request).execute().use { response ->
                    chain?.addAll(response.handshake?.peerCertificates.orEmpty().filterIsInstance<X509Certificate>())
                    when {
                        response.code == 401 -> throw UnauthorizedException()
                        !response.isSuccessful -> throw ServerException(response.code, url.encodedPath)
                        else -> response.body.string()
                    }
                }
            } catch (e: IOException) {
                if (e !is UnauthorizedException && e !is ServerException) {
                    if (certificateNotOfLogin(e)) throw CertificateChangedException()
                    if (certificateRefused(e)) throw UntrustedCertificateException()
                }
                throw e
            }
        }

    companion object {
        const val PAGE_SIZE = 200
        private const val LIBRARY_FIELDS = "Overview,MediaSources,Genres,DateCreated,RecursiveItemCount,ChildCount"
        private val JSON = "application/json".toMediaType()

        /**
         * The server answers a path from its web root: Movies' begins with the gateway prefix
         * `/movies`, Jellyfin's with `/videos`. Either way it belongs to the server logged in to;
         * an absolute URL to somewhere else is refused.
         */
        /** Bits per second to ask for: enough for 4K, for 1080p or for 720p, by the widest picture the decoder takes (0 = unknown). */
        internal fun bitrateFor(maxWidth: Int): Int = when {
            maxWidth == 0 || maxWidth >= 3840 -> 40_000_000
            maxWidth >= 1920 -> 10_000_000
            else -> 4_000_000
        }

        internal fun resolve(baseUrl: String, relative: String): String? {
            val base = baseUrl.trimEnd('/').toHttpUrl()
            if (!relative.startsWith("/") || relative.startsWith("//")) return null
            val basePath = base.encodedPath.trimEnd('/')
            val origin = base.newBuilder().encodedPath("/").build().toString().trimEnd('/')
            return if (basePath.isNotEmpty() && (relative == basePath || relative.startsWith("$basePath/"))) origin + relative
            else origin + basePath + relative
        }

        private fun seg(s: String): String = s.replace("/", "").replace("?", "").replace("#", "")

        private fun url(baseUrl: String, path: String, query: HttpUrl.Builder.() -> Unit): HttpUrl =
            (baseUrl.trimEnd('/') + path).toHttpUrl().newBuilder().apply(query).build()

        /**
         * Base URLs to try for what a person typed, most likely first. Without a scheme it is
         * http (the ZimaOS gateway speaks http on the LAN); a bare host is tried as a ZimaOS box
         * (`/movies`) first and as a Jellyfin server of its own second.
         */
        /**
         * Why no candidate answered, of the errors in the order they were tried: a certificate the
         * device does not trust is the reason to tell, even when a later candidate (another port)
         * failed in another way; else the last error.
         */
        /**
         * Was a certificate refused somewhere in [e]? A name with two addresses (IPv6 and IPv4)
         * shows why the errors attached to it count too: OkHttp tries the second address after the
         * first refused the connection and throws the first error, with the second attached
         * (`localhost` as ::1 and 127.0.0.1 on GitHub's runners, 2026-11-13). A handshake that fails
         * for another reason (no common protocol) is not a refused certificate.
         */
        internal fun certificateRefused(e: Throwable): Boolean = within(e) { it is SSLPeerUnverifiedException || it is CertificateException }

        /** A certificate other than the one of the login ([ScopedTrust]), in [e] as in [certificateRefused]. */
        internal fun certificateNotOfLogin(e: Throwable): Boolean = within(e) { it is CertificateNotOfLoginException }

        private fun within(e: Throwable, test: (Throwable) -> Boolean): Boolean {
            val seen = HashSet<Throwable>()
            val todo = ArrayDeque(listOf(e))
            while (todo.isNotEmpty() && seen.size < 32) {
                val t = todo.removeFirst()
                if (!seen.add(t)) continue
                if (test(t)) return true
                t.cause?.let(todo::add)
                todo.addAll(t.suppressed)
            }
            return false
        }

        internal fun reason(errors: List<IOException>): IOException =
            errors.firstOrNull { it is UntrustedCertificateException } ?: errors.lastOrNull() ?: IOException("no address")

        fun candidates(input: String): List<String> {
            var text = input.trim()
            if (text.isEmpty()) return emptyList()
            if (!text.contains("://")) text = "http://$text"
            val parsed = try {
                text.toHttpUrl()
            } catch (_: IllegalArgumentException) {
                return emptyList()
            }
            val root = parsed.toString().trimEnd('/')
            if (parsed.encodedPath != "/" && parsed.encodedPath.isNotEmpty()) return listOf(root)
            val out = mutableListOf("$root/movies", root)
            // Jellyfin's own port, when none was typed.
            if (parsed.port == HttpUrl.defaultPort(parsed.scheme)) out.add(parsed.newBuilder().port(8096).build().toString().trimEnd('/'))
            return out
        }
    }
}
