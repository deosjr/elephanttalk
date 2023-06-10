package talk

import (
	"github.com/deosjr/elephanttalk/opencv"
	"github.com/deosjr/whistle/lisp"
)

// v2 version of claim/wish/when model
// samples (claims/wishes) are no longer the same: a claim is a hard assert into db,
// but a wish is a special kind of assertion to be used in 'when'
// example: when /someone/ wishes x: <code>
// after running fixpoint analysis once per frame, some claims are still picked up
// outside of db and executed upon, mostly illumination-related (blit)
// TODO: insertion of var 'this' does not work properly? execution context is not correct
// solution: insert (define this ?id) at start of each codeblock?
func LoadRealTalk(l lisp.Lisp) {
	l.Eval(`(define-syntax claim
              (syntax-rules (dl_assert this claims list)
                ((_ id attr value) (begin
                 (dl_assert this 'claims (list id attr value))
                 (dl_assert id attr value)))))`)

	l.Eval(`(define-syntax wish
              (syntax-rules (dl_assert this wishes)
                ((_ x) (dl_assert this 'wishes (quote x)))))`)
	// 'when' makes a rule and includes code execution
	// this code execution is handled by hacking into the datalog implementation (see below)
	// code can include further claims/wishes or even other when-statements
	// NOTE: for now, this is going to be executing on every fixpoint iteration that matches,
	// so it better be idempotent / not too inefficient!
	// if conditions match, assert a fact (?id 'code ?code) where ?code already has vars replaced
	// (when (is-a (unquote ?page) window) do (wish ((unquote ?page) highlighted blue)))
	l.Eval(`(define-syntax when
              (syntax-rules (wishes do code this dl_rule :- begin)
                ((_ (condition ...) do statement ...)
                 (dl_rule (code this (begin statement ...)) :- condition ...))
                ((_ someone wishes w do statement ...)
                 (dl_rule (code this (begin statement ...)) :- (wishes someone w)))))`)

	// overwrite part of datalog naive fixpoint implementation
	// to include code execution in when-blocks!
	// NOTE: assumes all rules are ((code id (stmt ...)) :- condition ...)
	// runs each newly found code to run using map eval
	// NOTE: order is _not_ guaranteed but once code includes bindings, so same rule should only run once per set of bindings
	// (due to key equivalence being checked on the FULL sexpression code)
	// TODO: do we even need to update indices?
	l.Eval(`(define dl_fixpoint_iterate (lambda ()
       (let ((new (hashmap-keys (set_difference (foldl (lambda (x y) (set-extend! y x)) (map dl_apply_rule dl_rdb) (make-hashmap)) dl_idb))))
         (set-extend! dl_idb new)
         (map dl_update_indices new)
         (map (lambda (c) (eval (car (cdr (cdr c))))) new)
         (if (not (null? new)) (dl_fixpoint_iterate)))))`)

	opencv.Load(l.Env)
}

