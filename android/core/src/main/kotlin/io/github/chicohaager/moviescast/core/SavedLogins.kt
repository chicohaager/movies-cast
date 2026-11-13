package io.github.chicohaager.moviescast.core

import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/**
 * One login the device keeps: a server and an account there. [sealedToken] is the token as the
 * device's key store encrypted it; this class never sees it in the clear.
 */
@Serializable
data class SavedLogin(
    val baseUrl: String,
    val userId: String,
    val userName: String,
    val sealedToken: String,
    /** The server's own name, for the list; may be empty. */
    val serverName: String = "",
    /** What its certificates must come from ([ScopedTrust]): "system", "user:<sha-256>", or "" before there was a binding. */
    val trust: String = "",
) {
    /** `host:port` of the server, as [ScopedTrust] names it; "" for an address that cannot be read. */
    val server: String get() = SavedLogins.serverOf(baseUrl)

    /** "account · server name"; the account alone when the server told no name. */
    val title: String get() = if (serverName.isBlank()) userName else "$userName · $serverName"

    override fun toString(): String = "SavedLogin(baseUrl=$baseUrl, userId=$userId, userName=$userName)"
}

/**
 * The logins the device keeps and which of them is in use ([current], -1 for none). As in
 * Jellyfin's own Android clients a person can be logged in at several servers, or with several
 * accounts at one, and change between them without typing a password again.
 */
@Serializable
data class SavedLogins(val logins: List<SavedLogin> = emptyList(), val current: Int = -1) {

    val active: SavedLogin? get() = logins.getOrNull(current)

    /** Adds [login] and puts it in use; a login of the same account at the same server is replaced where it stands. */
    fun with(login: SavedLogin): SavedLogins {
        val at = logins.indexOfFirst { it.baseUrl == login.baseUrl && it.userId == login.userId }
        return if (at >= 0) SavedLogins(logins.toMutableList().also { it[at] = login }, at) else SavedLogins(logins + login, logins.size)
    }

    /** Puts the login at [index] in use; an index that does not exist changes nothing. */
    fun chosen(index: Int): SavedLogins = if (index in logins.indices) copy(current = index) else this

    /** Drops the login at [index]. The one in use stays in use; if it was the one dropped, none is. */
    fun without(index: Int): SavedLogins {
        if (index !in logins.indices) return this
        val next = when {
            index == current -> -1
            index < current -> current - 1
            else -> current
        }
        return SavedLogins(logins.filterIndexed { i, _ -> i != index }, next)
    }

    /** The binding of the logins at [server] (`host:port`): that of the one added last, null when there is none. */
    fun trustOf(server: String): String? = logins.lastOrNull { it.server == server }?.trust

    /** Every login at [server] gets the binding [trust] (two accounts at one server share the certificate). */
    fun bound(server: String, trust: String): SavedLogins = copy(logins = logins.map { if (it.server == server) it.copy(trust = trust) else it })

    fun encode(): String = json.encodeToString(serializer(), this)

    companion object {
        private val json = Json { ignoreUnknownKeys = true }

        /** `host:port` of a base URL (the scheme's port when none is written); "" when it cannot be read. */
        fun serverOf(baseUrl: String): String = baseUrl.toHttpUrlOrNull()?.let { "${it.host}:${it.port}" }.orEmpty()

        /** What [encode] wrote; a text that is not that gives an empty list, and a [current] outside the list gives none in use. */
        fun decode(text: String?): SavedLogins {
            if (text.isNullOrBlank()) return SavedLogins()
            val read = try {
                json.decodeFromString(serializer(), text)
            } catch (_: SerializationException) {
                return SavedLogins()
            } catch (_: IllegalArgumentException) {
                return SavedLogins()
            }
            return if (read.current in read.logins.indices) read else read.copy(current = -1)
        }
    }
}
