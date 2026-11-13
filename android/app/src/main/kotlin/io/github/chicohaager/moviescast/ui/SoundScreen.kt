package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.core.Passthrough

/** The names on the screen: product names, the same in every language. */
private fun Passthrough.Format.title(): String = when (this) {
    Passthrough.Format.AC3 -> "Dolby Digital (AC3)"
    Passthrough.Format.EAC3 -> "Dolby Digital Plus (E-AC3)"
    Passthrough.Format.DTS -> "DTS"
    Passthrough.Format.TRUEHD -> "Dolby TrueHD"
}

/**
 * "Sound": per format, whether it may go out unchanged over HDMI when the device says it can (as Jellyfin's
 * Android TV app and Kodi offer it), or is decoded by the app. A tap (OK on the remote) switches.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SoundScreen(off: Set<Passthrough.Format>, onBack: () -> Unit, onChange: (Set<Passthrough.Format>) -> Unit) {
    var current by remember { mutableStateOf(off) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.sound), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
            )
        },
    ) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(stringResource(R.string.sound_intro), style = MaterialTheme.typography.bodyMedium)
            for (format in Passthrough.Format.entries) {
                val decoded = format in current
                OutlinedButton(
                    onClick = {
                        current = if (decoded) current - format else current + format
                        onChange(current)
                    },
                    shape = RoundedCornerShape(12.dp),
                    modifier = Modifier.fillMaxWidth().testTag("sound_${format.key}"),
                ) {
                    Column(Modifier.fillMaxWidth()) {
                        Text(format.title(), style = MaterialTheme.typography.bodyLarge)
                        Text(
                            stringResource(if (decoded) R.string.sound_decode else R.string.sound_pass_on),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}
