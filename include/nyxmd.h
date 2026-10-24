#ifndef NYXMD_H
#define NYXMD_H
/*
 * NYX-MD — interface C, version d'ABI 1.
 * nyxmd_call : requête JSON UTF-8 terminée par \0 → réponse JSON allouée.
 * Toute réponse non nulle doit être libérée avec nyxmd_free. Voir docs/API.md.
 */
char *nyxmd_call(const char *request_json);
void nyxmd_free(char *response);
#endif
