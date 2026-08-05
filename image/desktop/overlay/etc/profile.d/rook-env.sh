# Export the saved backend key into every login shell, so `rook` finds it
# without the researcher re-entering it. Written by `rook-setup` into the
# persistent ~/.config/rook directory. Sourced, never run.
if [ -r "${HOME}/.config/rook/env" ]; then
    set -a
    . "${HOME}/.config/rook/env"
    set +a
fi
