package io.github.chicohaager.moviescast

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.MoviesClient
import io.github.chicohaager.moviescast.core.ServerException
import io.github.chicohaager.moviescast.core.ServerInfo
import io.github.chicohaager.moviescast.core.Session
import io.github.chicohaager.moviescast.core.UnauthorizedException
import io.github.chicohaager.moviescast.core.UnexpectedAnswerException
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
    data class Detail(val id: String) : Screen
    data object About : Screen
    data object Licences : Screen
    data class Licence(val id: String) : Screen
}

data class UiState(
    val stack: List<Screen> = listOf(Screen.Login),
    val userName: String = "",
    val server: ServerInfo? = null,
    val busy: Boolean = false,
    val error: UiError? = null,
    val library: List<MediaItem> = emptyList(),
    val resume: List<MediaItem> = emptyList(),
    val loaded: Boolean = false,
    val query: String = "",
    val detail: MediaItem? = null,
    val episodes: List<MediaItem> = emptyList(),
) {
    val screen: Screen get() = stack.last()

    val shown: List<MediaItem>
        get() = if (query.isBlank()) library else library.filter { it.name.contains(query.trim(), ignoreCase = true) }
}

class AppViewModel(private val app: MoviesCastApp) : ViewModel() {
    private val client: MoviesClient get() = app.client
    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state.asStateFlow()
    private var loading: Job? = null

    val lastServer: String get() = app.store.lastServer
    val lastUser: String get() = app.store.lastUser

    init {
        app.session?.let { s ->
            _state.update { it.copy(stack = listOf(Screen.Library), userName = s.userName) }
            refresh()
        }
    }

    fun login(server: String, user: String, password: String) {
        if (_state.value.busy) return
        if (MoviesClient.candidates(server).isEmpty()) {
            _state.update { it.copy(error = UiError.Address) }
            return
        }
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val found = try {
                client.discover(server)
            } catch (_: IOException) {
                _state.update { it.copy(busy = false, error = UiError.NoServer) }
                return@launch
            }
            try {
                val session = client.login(found.first, user.trim(), password)
                app.store.rememberTyped(server.trim())
                app.setSession(session)
                _state.update {
                    UiState(stack = listOf(Screen.Library), userName = session.userName, server = found.second)
                }
                refresh()
            } catch (e: IOException) {
                _state.update { it.copy(busy = false, error = if (e is UnauthorizedException) UiError.Login else toError(e)) }
            }
        }
    }

    /** Reads the library and the resume row again; also called when the player closes. */
    fun refresh() {
        val session = app.session ?: return
        loading?.cancel()
        _state.update { it.copy(busy = true, error = null) }
        loading = viewModelScope.launch {
            try {
                val (library, resume) = coroutineScope {
                    val all = async { client.library(session) }
                    val cont = async { client.resume(session) }
                    all.await() to cont.await()
                }
                _state.update { it.copy(busy = false, library = library, resume = resume, loaded = true) }
                (_state.value.screen as? Screen.Detail)?.let { loadDetail(session, it.id) }
            } catch (e: IOException) {
                fail(e)
            }
        }
    }

    /** The person refused access to the home network; nothing on the LAN can be reached. */
    fun localNetworkDenied() = _state.update { it.copy(busy = false, error = UiError.LocalNetwork) }

    fun search(query: String) = _state.update { it.copy(query = query) }

    fun open(item: MediaItem) {
        val session = app.session ?: return
        _state.update { it.copy(stack = it.stack + Screen.Detail(item.id), detail = item, episodes = emptyList(), error = null) }
        viewModelScope.launch {
            try {
                loadDetail(session, item.id)
            } catch (e: IOException) {
                fail(e)
            }
        }
    }

    private suspend fun loadDetail(session: Session, id: String) {
        val full = client.item(session, id)
        val episodes = if (full.type == "Series") client.episodes(session, id) else emptyList()
        _state.update { if ((it.screen as? Screen.Detail)?.id == id) it.copy(detail = full, episodes = episodes) else it }
    }

    /** Opens a page on top of the current one: About, the licences, one licence. */
    fun show(screen: Screen) = _state.update { it.copy(stack = it.stack + screen, error = null) }

    /** Returns false when there is nothing to go back to (the activity then closes). */
    fun back(): Boolean {
        val stack = _state.value.stack
        if (stack.size <= 1) return false
        _state.update { it.copy(stack = stack.dropLast(1), error = null) }
        return true
    }

    fun logout() {
        val session = app.session
        app.setSession(null)
        _state.value = UiState()
        if (session != null) {
            app.appScope.launch {
                try {
                    client.logout(session)
                } catch (_: IOException) {
                    // the local login is gone either way
                }
            }
        }
    }

    private fun fail(e: IOException) {
        if (e is UnauthorizedException) {
            app.setSession(null)
            _state.value = UiState(error = UiError.Session)
        } else {
            _state.update { it.copy(busy = false, error = toError(e)) }
        }
    }

    private fun toError(e: IOException): UiError = when {
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
