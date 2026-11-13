package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiError
import io.github.chicohaager.moviescast.UiState
import io.github.chicohaager.moviescast.core.MediaItem

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibraryScreen(
    state: UiState,
    onSearch: (String) -> Unit,
    onOpen: (MediaItem) -> Unit,
    onRefresh: () -> Unit,
    onLogout: () -> Unit,
    onOpenSettings: () -> Unit,
    onAbout: () -> Unit,
) {
    val searchField = rememberRemoteField()
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.app_name), maxLines = 1)
                        if (state.userName.isNotEmpty()) {
                            Text(
                                state.userName,
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                },
                actions = {
                    TextButton(onClick = onRefresh, modifier = Modifier.testTag("reload")) { Text(stringResource(R.string.library_refresh)) }
                    // Three words do not fit beside the title in every language (French, measured 2026-11-12).
                    var menu by rememberSaveable { mutableStateOf(false) }
                    var languages by rememberSaveable { mutableStateOf(false) }
                    val menuName = stringResource(R.string.menu)
                    TextButton(
                        onClick = { menu = true },
                        modifier = Modifier.testTag("menu").semantics { contentDescription = menuName },
                    ) { Text("⋮", style = MaterialTheme.typography.titleLarge) }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.language)) },
                            onClick = { menu = false; languages = true },
                            modifier = Modifier.testTag("language"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.about)) },
                            onClick = { menu = false; onAbout() },
                            modifier = Modifier.testTag("about"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.logout)) },
                            onClick = { menu = false; onLogout() },
                            modifier = Modifier.testTag("logout"),
                        )
                    }
                    if (languages) LanguageDialog { languages = false }
                },
            )
        },
    ) { padding ->
        PullToRefreshBox(isRefreshing = state.busy, onRefresh = onRefresh, modifier = Modifier.padding(padding).fillMaxSize()) {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(minSize = 116.dp),
                contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 24.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
                modifier = Modifier.fillMaxSize().testTag("library"),
            ) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    OutlinedTextField(
                        value = state.query,
                        onValueChange = onSearch,
                        label = { Text(stringResource(R.string.library_search)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                        readOnly = searchField.readOnly,
                        modifier = Modifier.fillMaxWidth().then(searchField.modifier).testTag("search"),
                    )
                }
                state.error?.let { error ->
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(errorText(error), color = MaterialTheme.colorScheme.error)
                            Button(onClick = onRefresh) { Text(stringResource(R.string.retry)) }
                            if (error == UiError.LocalNetwork) {
                                OutlinedButton(onClick = onOpenSettings) { Text(stringResource(R.string.open_settings)) }
                            }
                        }
                    }
                }
                if (state.query.isBlank() && state.resume.isNotEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) { Heading(stringResource(R.string.library_continue)) }
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            items(state.resume, key = { it.id }) { item ->
                                Tile(item, Modifier.width(132.dp), onOpen, showProgress = true)
                            }
                        }
                    }
                }
                if (state.loaded) {
                    item(span = { GridItemSpan(maxLineSpan) }) { Heading(stringResource(R.string.library_all)) }
                    if (state.shown.isEmpty()) {
                        item(span = { GridItemSpan(maxLineSpan) }) {
                            Text(
                                if (state.query.isBlank()) stringResource(R.string.library_empty)
                                else stringResource(R.string.library_no_match, state.query.trim()),
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
                items(state.shown, key = { it.id }) { item -> Tile(item, Modifier, onOpen, showProgress = false) }
            }
        }
    }
}

@Composable
private fun Heading(text: String) {
    Text(text, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 4.dp))
}

@Composable
private fun Tile(item: MediaItem, modifier: Modifier, onOpen: (MediaItem) -> Unit, showProgress: Boolean) {
    var focused by remember { mutableStateOf(false) }
    Column(
        modifier
            .onFocusChanged { focused = it.isFocused }
            // A remote control has no pointer: the tile in focus gets a frame.
            .border(3.dp, if (focused) MaterialTheme.colorScheme.primary else Color.Transparent, RoundedCornerShape(12.dp))
            .padding(3.dp)
            .clip(RoundedCornerShape(10.dp))
            .clickable { onOpen(item) }
            .testTag("tile"),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Box(contentAlignment = Alignment.BottomCenter) {
            Poster(item, Modifier.fillMaxWidth())
            if (showProgress && item.runtimeSeconds > 0) {
                LinearProgressIndicator(
                    progress = { (item.resumeSeconds.toFloat() / item.runtimeSeconds).coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }
        Text(item.name, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (item.year > 0) {
            Text(item.year.toString(), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
