package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiError
import io.github.chicohaager.moviescast.UiState

@Composable
fun LoginScreen(
    state: UiState,
    lastServer: String,
    lastUser: String,
    onLogin: (String, String, String) -> Unit,
    onOpenSettings: () -> Unit,
) {
    var server by rememberSaveable { mutableStateOf(lastServer) }
    var user by rememberSaveable { mutableStateOf(lastUser) }
    // Not saved across process death on purpose: a password does not belong in the saved state.
    var password by androidx.compose.runtime.remember { mutableStateOf("") }
    val ready = server.isNotBlank() && user.isNotBlank() && !state.busy
    val submit = { if (ready) onLogin(server, user, password) }
    val serverField = rememberRemoteField()
    val userField = rememberRemoteField()
    val passwordField = rememberRemoteField()

    Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Box(Modifier.fillMaxSize().safeDrawingPadding().imePadding(), contentAlignment = Alignment.Center) {
            Column(
                Modifier.widthIn(max = 440.dp).fillMaxWidth().verticalScroll(rememberScrollState()).padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                Text(stringResource(R.string.app_name), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                Text(stringResource(R.string.login_title), style = MaterialTheme.typography.headlineMedium)
                Text(
                    stringResource(R.string.login_hint),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = server,
                    onValueChange = { server = it },
                    label = { Text(stringResource(R.string.login_server)) },
                    placeholder = { Text(stringResource(R.string.login_server_example)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                        keyboardType = KeyboardType.Uri,
                        imeAction = ImeAction.Next,
                    ),
                    readOnly = serverField.readOnly,
                    modifier = Modifier.fillMaxWidth().then(serverField.modifier).testTag("server"),
                )
                OutlinedTextField(
                    value = user,
                    onValueChange = { user = it },
                    label = { Text(stringResource(R.string.login_user)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        autoCorrectEnabled = false,
                        imeAction = ImeAction.Next,
                    ),
                    readOnly = userField.readOnly,
                    modifier = Modifier.fillMaxWidth().then(userField.modifier).testTag("user"),
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.login_password)) },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                    keyboardActions = KeyboardActions(onGo = { submit() }),
                    readOnly = passwordField.readOnly,
                    modifier = Modifier.fillMaxWidth().then(passwordField.modifier).testTag("password"),
                )
                state.error?.let {
                    Text(errorText(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                    if (it == UiError.LocalNetwork) {
                        OutlinedButton(onClick = onOpenSettings, modifier = Modifier.testTag("settings")) {
                            Text(stringResource(R.string.open_settings))
                        }
                    }
                }
                Button(onClick = submit, enabled = ready, modifier = Modifier.fillMaxWidth().height(52.dp).testTag("login")) {
                    if (state.busy) {
                        CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                    } else {
                        Text(stringResource(R.string.login_button))
                    }
                }
                LanguageButton(Modifier.align(Alignment.CenterHorizontally))
            }
        }
    }
}
