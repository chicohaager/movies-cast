package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import android.content.res.Configuration
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import io.github.chicohaager.moviescast.MoviesCastApp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiError
import io.github.chicohaager.moviescast.core.MediaItem

@Composable
fun errorText(error: UiError): String = when (error) {
    UiError.Address -> stringResource(R.string.error_address)
    UiError.NoServer -> stringResource(R.string.error_no_server)
    UiError.Login -> stringResource(R.string.error_login)
    UiError.Session -> stringResource(R.string.error_session)
    UiError.Network -> stringResource(R.string.error_network)
    UiError.LocalNetwork -> stringResource(R.string.error_local_network)
    UiError.Answer -> stringResource(R.string.error_answer)
    is UiError.Server -> stringResource(R.string.error_server, error.status)
}

/** 1:02:03 or 12:03. */
fun clock(seconds: Long): String {
    val h = seconds / 3600
    val m = seconds % 3600 / 60
    val s = seconds % 60
    return if (h > 0) "%d:%02d:%02d".format(h, m, s) else "%d:%02d".format(m, s)
}

/** The poster of an item, or its name on a plain tile when the server has none. */
@Composable
fun Poster(item: MediaItem, modifier: Modifier = Modifier, widthPx: Int = 400) {
    val app = LocalContext.current.applicationContext as MoviesCastApp
    val session = app.session
    Box(
        modifier
            .aspectRatio(2f / 3f)
            .clip(RoundedCornerShape(10.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant),
        contentAlignment = Alignment.Center,
    ) {
        if (item.hasPoster && session != null) {
            AsyncImage(
                model = app.client.posterUrl(session, item.id, widthPx),
                contentDescription = item.name,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            Text(
                item.name,
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                maxLines = 4,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.fillMaxWidth().padding(8.dp),
            )
        }
    }
}

/** What a text field needs to be usable with a TV remote; see [rememberRemoteField]. */
class RemoteField(val readOnly: Boolean, val modifier: Modifier)

/**
 * Makes a one-line text field usable with a remote control.
 *
 * Measured 2026-11-12 on an Android TV emulator: a text field that gets the focus opens the
 * on-screen keyboard at once, and the keyboard then takes the arrow keys, so the viewer cannot
 * walk past the field. On a TV the field therefore stays read-only (no keyboard) until OK is
 * pressed on it. On every device up and down leave the field, as a one-line field has no line
 * to move to. Phones and tablets keep the usual behaviour: touching the field opens the keyboard.
 */
@Composable
fun rememberRemoteField(): RemoteField {
    val tv = (LocalConfiguration.current.uiMode and Configuration.UI_MODE_TYPE_MASK) == Configuration.UI_MODE_TYPE_TELEVISION
    var editing by remember { mutableStateOf(false) }
    val focus = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val modifier = Modifier
        .onFocusChanged { if (!it.hasFocus) editing = false }
        .onPreviewKeyEvent { event ->
            val down = event.type == KeyEventType.KeyDown
            when (event.key) {
                Key.DirectionDown -> { if (down) focus.moveFocus(FocusDirection.Down); true }
                Key.DirectionUp -> { if (down) focus.moveFocus(FocusDirection.Up); true }
                Key.DirectionCenter, Key.Enter, Key.NumPadEnter ->
                    if (tv && !editing) {
                        if (!down) {
                            editing = true
                            keyboard?.show()
                        }
                        true
                    } else {
                        false
                    }
                else -> false
            }
        }
    return RemoteField(readOnly = tv && !editing, modifier = modifier)
}
