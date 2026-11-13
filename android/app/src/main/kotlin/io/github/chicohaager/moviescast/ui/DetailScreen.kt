package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiState
import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.Track

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DetailScreen(
    state: UiState,
    onBack: () -> Unit,
    onOpen: (MediaItem) -> Unit,
    onPlay: (MediaItem, Long) -> Unit,
    onPlayed: (MediaItem, Boolean) -> Unit,
) {
    val item = state.detail ?: return
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(item.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
            )
        },
    ) { padding ->
        BoxWithConstraints(Modifier.padding(padding).fillMaxSize()) {
            val wide = maxWidth >= 600.dp
            if (wide) {
                Row(Modifier.fillMaxSize().padding(horizontal = 24.dp), horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                    Poster(item, Modifier.width(260.dp).padding(top = 8.dp), widthPx = 600)
                    Column(Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(bottom = 24.dp)) {
                        Facts(state, item, onOpen, onPlay, onPlayed)
                    }
                }
            } else {
                Column(
                    Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    Poster(item, Modifier.width(180.dp), widthPx = 600)
                    Facts(state, item, onOpen, onPlay, onPlayed)
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Facts(state: UiState, item: MediaItem, onOpen: (MediaItem) -> Unit, onPlay: (MediaItem, Long) -> Unit, onPlayed: (MediaItem, Boolean) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Text(item.name, style = MaterialTheme.typography.headlineSmall)
        val line = listOfNotNull(
            item.seriesName.takeIf { it.isNotEmpty() },
            item.year.takeIf { it > 0 }?.toString(),
            (item.runtimeSeconds / 60).takeIf { it > 0 }?.let { stringResource(R.string.detail_minutes, it.toInt()) },
            item.rating.takeIf { it.isNotEmpty() },
            item.genres.take(3).joinToString(", ").takeIf { it.isNotEmpty() },
        ).joinToString(" · ")
        if (line.isNotEmpty()) Text(line, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodyMedium)

        if (item.playable) {
            // With a remote control the first OK should play: the main button takes the focus
            // when the page opens (measured 2026-11-12 on Android TV: nothing was in focus).
            val mainButton = remember { FocusRequester() }
            LaunchedEffect(item.id) { runCatching { mainButton.requestFocus() } }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (item.resumable) {
                    Button(onClick = { onPlay(item, item.resumeSeconds) }, modifier = Modifier.focusRequester(mainButton).testTag("resume")) {
                        Text(stringResource(R.string.play_resume, clock(item.resumeSeconds)))
                    }
                    OutlinedButton(onClick = { onPlay(item, 0) }, modifier = Modifier.testTag("restart")) {
                        Text(stringResource(R.string.play_restart))
                    }
                } else {
                    Button(onClick = { onPlay(item, 0) }, modifier = Modifier.focusRequester(mainButton).testTag("play")) { Text("▶  " + stringResource(R.string.play)) }
                }
                OutlinedButton(onClick = { onPlayed(item, !item.played) }, modifier = Modifier.testTag("played")) {
                    Text(if (item.played) "✓  " + stringResource(R.string.mark_unwatched) else stringResource(R.string.mark_watched))
                }
            }
        }

        if (item.overview.isNotEmpty()) Text(item.overview, style = MaterialTheme.typography.bodyLarge)

        if (item.playable) {
            val file = listOfNotNull(
                item.container.takeIf { it.isNotEmpty() }?.uppercase(),
                item.videoCodec.takeIf { it.isNotEmpty() }?.uppercase(),
                if (item.width > 0 && item.height > 0) "${item.width}×${item.height}" else null,
                item.sizeBytes.takeIf { it > 0 }?.let { fileSize(it) },
            ).joinToString(" · ")
            Fact(stringResource(R.string.detail_file), file)
            Fact(stringResource(R.string.detail_audio), item.audio.joinToString("\n") { trackLine(it) })
            Fact(stringResource(R.string.detail_subtitles), item.subtitles.joinToString(", ") { it.title.ifEmpty { it.codec } })
        }

        if (state.episodes.isNotEmpty()) {
            Text(stringResource(R.string.episodes), style = MaterialTheme.typography.titleMedium)
            Column {
                var season = -1
                for (episode in state.episodes) {
                    if (episode.season != season) {
                        season = episode.season
                        Text(
                            if (season == 0) stringResource(R.string.season_specials) else stringResource(R.string.season_label, season),
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.padding(top = 18.dp, bottom = 6.dp).testTag("season"),
                        )
                    }
                    HorizontalDivider()
                    Text(
                        (if (episode.played) "✓  " else "") + stringResource(R.string.episode_label, episode.season, episode.episode, episode.name),
                        modifier = Modifier.fillMaxWidth().clickable { onOpen(episode) }.padding(vertical = 14.dp).testTag("episode"),
                    )
                }
            }
        }
    }
}

/** 2.3 GB, 740 MB. */
internal fun fileSize(bytes: Long): String =
    if (bytes >= 1_000_000_000) "%.1f GB".format(bytes / 1e9) else "%d MB".format((bytes / 1_000_000).coerceAtLeast(1))

private fun trackLine(track: Track): String {
    val title = track.title.ifEmpty { track.language }
    val codec = track.codec.uppercase()
    return when {
        title.isEmpty() -> codec
        title.contains(codec, ignoreCase = true) -> title
        else -> "$title ($codec)"
    }
}

@Composable
private fun Fact(label: String, value: String) {
    if (value.isEmpty()) return
    Column {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, style = MaterialTheme.typography.bodyMedium)
    }
}
