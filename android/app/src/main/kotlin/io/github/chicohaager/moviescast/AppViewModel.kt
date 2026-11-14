package io.github.chicohaager.moviescast

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import io.github.chicohaager.moviescast.core.HomeLayout
import io.github.chicohaager.moviescast.core.TechFacts
import io.github.chicohaager.moviescast.core.LibraryFilter
import io.github.chicohaager.moviescast.core.LibrarySort
import io.github.chicohaager.moviescast.core.FoundServer
import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.MediaLibrary
import io.github.chicohaager.moviescast.core.MoviesClient
import io.github.chicohaager.moviescast.core.SavedLogin
import io.github.chicohaager.moviescast.core.ServerException
import io.github.chicohaager.moviescast.core.ServerInfo
import io.github.chicohaager.moviescast.core.Session
import io.github.chicohaager.moviescast.core.UnauthorizedException
import io.github.chicohaager.moviescast.core.UnexpectedAnswerException
import io.github.chicohaager.moviescast.core.UntrustedCertificateException
import io.github.chicohaager.moviescast.core.CertificateChangedException
import io.github.chicohaager.moviescast.core.CertificateNeedsLoginException
import io.github.chicohaager.moviescast.core.SavedLogins
import java.security.cert.X509Certificate
import io.github.chicohaager.moviescast.core.after
import io.github.chicohaager.moviescast.core.arranged
import java.io.IOException
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** What went wrong, in a form a screen can put into words. */
sealed interface UiError {
    data object Address : UiError
    data object NoServer : UiError
    data object Certificate : UiError
    data object CertificateChanged : UiError
    data object CertificateNeedsLogin : UiError
    data object Login : UiError
    data object Session : UiError
    data object Network : UiError
    data object LocalNetwork : UiError
    data object Answer : UiError
    data class Server(val status: Int) : UiError
}

sealed interface Screen {
    data object Login : Screen
    data object Library : Screen
    data object Accounts : Screen
    data class Detail(val id: String) : Screen
    data object About : Screen
    data object Licences : Screen
    data class Licence(val id: String) : Screen
    data object Privacy : Screen
    data object Sound : Screen
    data object Home : Screen
}

data class UiState(
    val stack: List<Screen> = listOf(Screen.Login),
    val userName: String = "",
    /** The logins the device keeps (servers and accounts), and which is in use (-1: none). */
    val logins: List<SavedLogin> = emptyList(),
    val current: Int = -1,
    val server: ServerInfo? = null,
    val busy: Boolean = false,
    val error: UiError? = null,
    /** The home network is being searched for servers. */
    val searching: Boolean = false,
    /** What the last search found; null before the first search. */
    val found: List<FoundServer>? = null,
    val library: List<MediaItem> = emptyList(),
    /** The server's libraries ("Filme", "Serien" …) for the tiles under "My media". */
    val libraries: List<MediaLibrary> = emptyList(),
    val resume: List<MediaItem> = emptyList(),
    val loaded: Boolean = false,
    val query: String = "",
    val sort: LibrarySort = LibrarySort.NAME,
    val filter: LibraryFilter = LibraryFilter(),
    /** The rows above the library, as the person set them ("Home screen" in the menu). */
    val home: HomeLayout = HomeLayout(),
    val detail: MediaItem? = null,
    /** What [detail]'s file holds (server first, its header for the rest); null until known. */
    val tech: TechFacts? = null,
    val episodes: List<MediaItem> = emptyList(),
    /** For an episode: the episodes of its series that come after it; they play one after the other. */
    val following: List<MediaItem> = emptyList(),
) {
    val screen: Screen get() = stack.last()

    val shown: List<MediaItem>
        get() = library.arranged(query, sort, filter)

    /** True when the list shows everything in the server's order: only then "continue watching" stands above it. */
    val plain: Boolean get() = query.isBlank() && filter.kind == LibraryFilter.Kind.ALL
}

class AppViewModel(private val app: MoviesCastApp) : ViewModel() {
    private val client: MoviesClient get() = app.client
    private fun storedSort(): LibrarySort = LibrarySort.entries.firstOrNull { it.name == app.store.sort } ?: LibrarySort.NAME

    /**
     * The state to begin from: the library of the login in use; without one the list of the
     * logins kept, and the login page when there is none.
     */
    private fun start(error: UiError? = null): UiState {
        val kept = app.store.logins()
        val session = app.session
        val stack: List<Screen> = when {
            session != null -> listOf(Screen.Library)
            kept.logins.isEmpty() -> listOf(Screen.Login)
            else -> listOf(Screen.Accounts)
        }
        return UiState(stack = stack, userName = session?.userName.orEmpty(), logins = kept.logins, current = kept.current, error = error, sort = storedSort(), home = HomeLayout.decode(app.store.homeRows))
    }

    private val _state = MutableStateFlow(start())
    val state: StateFlow<UiState> = _state.asStateFlow()
    private var loading: Job? = null

    val lastServer: String get() = app.store.lastServer
    val lastUser: String get() = app.store.lastUser

    init {
        if (app.session != null) refresh()
    }

    fun login(server: String, user: String, password: String) {
        if (_state.value.busy) return
        if (MoviesClient.candidates(server).isEmpty()) {
            _state.update { it.copy(error = UiError.Address) }
            return
        }
        _state.update { it.copy(busy = true, error = null) }
        // While this login is made, an installed certificate may count for the address typed (and only for it).
        app.loggingIn = MoviesClient.candidates(server).map { SavedLogins.serverOf(it) }.toSet()
        viewModelScope.launch {
            try {
                loginNow(server, user, password)
            } finally {
                app.loggingIn = emptySet()
            }
        }
    }

    private suspend fun loginNow(server: String, user: String, password: String) {
        val found = try {
            client.discover(server)
        } catch (e: IOException) {
            _state.update { it.copy(busy = false, error = if (e is UntrustedCertificateException) UiError.Certificate else UiError.NoServer) }
            return
        }
        try {
            val chain = ArrayList<X509Certificate>()
            val session = client.login(found.first, user.trim(), password, chain)
            // bound to what issued the certificates of this answer; none found cannot be after a login the check let through
            val bound = app.trust.bindingOf(chain) ?: throw UntrustedCertificateException()
            app.store.rememberTyped(server.trim())
            loading?.cancel()
            app.setSession(session, found.second.name, bound)
            _state.value = start().copy(server = found.second)
            refresh()
        } catch (e: IOException) {
            _state.update { it.copy(busy = false, error = if (e is UnauthorizedException) UiError.Login else toError(e)) }
        }
    }

    /** Searches the home network for servers; the login page lists what answered. */
    fun findServers() {
        if (_state.value.searching || _state.value.busy) return
        _state.update { it.copy(searching = true, found = null, error = null) }
        viewModelScope.launch {
            val found = try {
                client.confirm(app.announced())
            } catch (e: IOException) {
                AppLog.w("search: failed (${e.javaClass.simpleName})")
                emptyList()
            }
            AppLog.i("search: ${found.size} server(s) answered")
            _state.update { it.copy(searching = false, found = found) }
        }
    }

    /** Reads the library and the resume row again; also called when the player closes. */
    fun refresh() {
        val session = app.session ?: return
        loading?.cancel()
        _state.update { it.copy(busy = true, error = null) }
        loading = viewModelScope.launch {
            try {
                val (library, resume, libraries) = coroutineScope {
                    val all = async { client.library(session) }
                    val cont = async { client.resume(session) }
                    // The tiles are a way into the library, not the library: a server that cannot list its libraries
                    // still shows everything, and the row is left out (logged, so it is not lost silently).
                    val views = async {
                        try {
                            client.libraries(session)
                        } catch (e: IOException) {
                            AppLog.w("libraries: ${e.javaClass.simpleName}")
                            emptyList()
                        }
                    }
                    Triple(all.await(), cont.await(), views.await())
                }
                _state.update { it.copy(busy = false, library = library, resume = resume, libraries = libraries, loaded = true) }
                (_state.value.screen as? Screen.Detail)?.let { loadDetail(session, it.id) }
            } catch (e: IOException) {
                fail(e, session)
            }
        }
    }

    /** The person refused access to the home network; nothing on the LAN can be reached. */
    fun localNetworkDenied() = _state.update { it.copy(busy = false, searching = false, error = UiError.LocalNetwork) }

    fun search(query: String) = _state.update { it.copy(query = query) }

    /** The order is kept for the next start; a filter is a look at the moment and is not. */
    fun sort(sort: LibrarySort) {
        app.store.sort = sort.name
        _state.update { it.copy(sort = sort) }
    }

    fun filter(filter: LibraryFilter) = _state.update { it.copy(filter = filter) }

    /** The whole library again: no search, no filter. */
    fun showAll() = _state.update { it.copy(query = "", filter = LibraryFilter()) }

    /** The home page's rows as the person set them; kept for the next start. */
    fun setHome(layout: HomeLayout) {
        app.store.homeRows = layout.encode()
        _state.update { it.copy(home = layout) }
    }

    fun open(item: MediaItem) {
        val session = app.session ?: return
        _state.update { it.copy(stack = it.stack + Screen.Detail(item.id), detail = item, episodes = emptyList(), following = emptyList(), tech = null, error = null) }
        viewModelScope.launch {
            try {
                loadDetail(session, item.id)
            } catch (e: IOException) {
                fail(e, session)
            }
        }
    }

    private suspend fun loadDetail(session: Session, id: String) {
        val full = client.item(session, id)
        val episodes = if (full.type == "Series") client.episodes(session, id) else emptyList()
        val following = if (full.type == "Episode" && full.seriesId.isNotEmpty()) client.episodes(session, full.seriesId).after(id) else emptyList()
        val server = TechFacts.of(full)
        val known = probed[id]?.let { server.mergedWith(it) } ?: server
        _state.update { if ((it.screen as? Screen.Detail)?.id == id) it.copy(detail = full, episodes = episodes, following = following, tech = known) else it }
        if (full.playable && server.incomplete && id !in probed && probing.add(id)) {
            // once per film while the app runs (a refresh during the read does not start a second): the file's header
            // for what the server does not name
            val read = try {
                FileProbe.read(app, app.mediaHttp, client.streamUrl(session, id))
            } finally {
                probing.remove(id)
            } ?: return
            probed[id] = read
            _state.update { if ((it.screen as? Screen.Detail)?.id == id) it.copy(tech = server.mergedWith(read)) else it }
        }
    }

    /** What the files' headers held, by item id, for this run of the app. */
    private val probed = HashMap<String, TechFacts>()
    private val probing = HashSet<String>()

    /** Opens a page on top of the current one: About, the licences, one licence. */
    fun show(screen: Screen) = _state.update { it.copy(stack = it.stack + screen, error = null) }

    /** Marks the item as watched or not; the library and the page are read again, so every mark on the screen is the server's. */
    fun setPlayed(item: MediaItem, played: Boolean) {
        val session = app.session ?: return
        viewModelScope.launch {
            try {
                client.setPlayed(session, item.id, played)
                refresh()
            } catch (e: IOException) {
                fail(e, session)
            }
        }
    }

    fun setFavorite(item: MediaItem, favorite: Boolean) {
        val session = app.session ?: return
        viewModelScope.launch {
            try {
                client.setFavorite(session, item.id, favorite)
                refresh()
            } catch (e: IOException) {
                fail(e, session)
            }
        }
    }

    /** Returns false when there is nothing to go back to (the activity then closes). */
    fun back(): Boolean {
        val stack = _state.value.stack
        if (stack.size <= 1) return false
        _state.update { it.copy(stack = stack.dropLast(1), error = null) }
        return true
    }

    /** Logs out of the login in use; the other logins stay and are then listed. */
    fun logout() {
        val session = app.session
        loading?.cancel()
        app.setSession(null)
        _state.value = start()
        if (session != null) tellLogout(session)
    }

    /** Puts another of the logins kept in use and reads its library. */
    fun switchTo(index: Int) {
        if (index == _state.value.current && app.session != null) {
            // already in use: back to its library
            if (!back()) _state.value = start()
            return
        }
        if (!app.switchTo(index)) {
            // its token cannot be read any more (the device's key is gone): it has to be typed again
            app.forget(index)
            val kept = app.store.logins()
            _state.update { it.copy(logins = kept.logins, current = kept.current, error = UiError.Session) }
            return
        }
        loading?.cancel()
        _state.value = start()
        refresh()
    }

    /** Forgets the login at [index] and logs it out at its server; the login in use stays in use unless it is this one. */
    fun forget(index: Int) {
        val wasCurrent = index == app.store.logins().current
        if (wasCurrent) loading?.cancel()
        val gone = app.forget(index)
        if (gone != null) tellLogout(gone)
        val kept = app.store.logins()
        if (wasCurrent) _state.value = start() else _state.update { it.copy(logins = kept.logins, current = kept.current) }
    }

    private fun tellLogout(session: Session) {
        app.appScope.launch {
            try {
                client.logout(session)
            } catch (_: IOException) {
                // the local login is gone either way
            }
        }
    }

    /** [of] is the login the failed request was made with: an answer that comes after a change of login must not touch the new one. */
    private fun fail(e: IOException, of: Session) {
        if (app.session !== of) return
        if (e is UnauthorizedException) {
            loading?.cancel()
            app.setSession(null)
            _state.value = start(UiError.Session)
        } else {
            _state.update { it.copy(busy = false, error = toError(e)) }
        }
    }

    private fun toError(e: IOException): UiError = when {
        e is CertificateChangedException -> UiError.CertificateChanged
        e is CertificateNeedsLoginException -> UiError.CertificateNeedsLogin
        e is UntrustedCertificateException -> UiError.Certificate
        e is ServerException -> UiError.Server(e.status)
        e is UnexpectedAnswerException -> UiError.Answer
        else -> UiError.Network
    }

    companion object {
        val Factory: ViewModelProvider.Factory = viewModelFactory {
            initializer { AppViewModel(this[ViewModelProvider.AndroidViewModelFactory.APPLICATION_KEY] as MoviesCastApp) }
        }
    }
}
